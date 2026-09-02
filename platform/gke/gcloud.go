package gke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	filePerm    = 0o644
	cronFixFile = "/etc/cron.d/cocoon-net-fix-alias"
)

// aliasEntry is one row from a GCE instance's nic0 aliasIpRanges list.
type aliasEntry struct {
	RangeName   string `json:"subnetworkRangeName"`
	IPCIDRRange string `json:"ipCidrRange"`
}

func (a aliasEntry) String() string {
	if a.RangeName == "" {
		return a.IPCIDRRange
	}
	return a.RangeName + ":" + a.IPCIDRRange
}

// runGcloud shells out to gcloud; the compute/apiv1 SDK migration is pending.
func runGcloud(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "gcloud", args...)
}

// ensureSecondaryRange reuses a range covering cidr, errors on one that does not, and creates it at cidr when absent (see docs/gke.md).
func ensureSecondaryRange(ctx context.Context, project, region, subnet, cidr string) error {
	logger := log.WithFunc("gke.ensureSecondaryRange")

	existing, err := describeSecondaryRange(ctx, project, region, subnet, aliasRangeName)
	if err != nil {
		return fmt.Errorf("describe subnet %s: %w", subnet, err)
	}

	if existing != "" {
		covers, err := platform.CIDRContainsCIDR(existing, cidr)
		if err != nil {
			return fmt.Errorf("compare existing range %s with %s: %w", existing, cidr, err)
		}
		if !covers {
			return fmt.Errorf(
				"secondary range %q on subnet %s is %s, which does not cover --subnet %s; expand the range or choose a --subnet inside it",
				aliasRangeName, subnet, existing, cidr,
			)
		}
		logger.Infof(ctx, "reusing secondary range %s=%s on subnet %s", aliasRangeName, existing, subnet)
		return nil
	}

	logger.Infof(
		ctx,
		"secondary range %s not found on subnet %s; creating with CIDR %s (multi-node clusters should pre-create a broader range, see docs/gke.md)",
		aliasRangeName, subnet, cidr,
	)
	if _, err := runGcloud(
		ctx,
		"compute", "networks", "subnets", "update", subnet,
		"--project", project, "--region", region,
		fmt.Sprintf("--add-secondary-ranges=%s=%s", aliasRangeName, cidr),
	); err != nil {
		return fmt.Errorf("create secondary range %s=%s: %w", aliasRangeName, cidr, err)
	}
	return nil
}

// describeSecondaryRange returns the named range's CIDR, or "" when absent.
func describeSecondaryRange(ctx context.Context, project, region, subnet, rangeName string) (string, error) {
	out, err := runGcloud(
		ctx,
		"compute", "networks", "subnets", "describe", subnet,
		"--project", project, "--region", region,
		"--format", "json",
	)
	if err != nil {
		return "", err
	}
	var desc struct {
		SecondaryIPRanges []struct {
			RangeName   string `json:"rangeName"`
			IPCIDRRange string `json:"ipCidrRange"`
		} `json:"secondaryIpRanges"`
	}
	if err := json.Unmarshal(out, &desc); err != nil {
		return "", fmt.Errorf("parse subnet describe: %w", err)
	}
	for _, r := range desc.SecondaryIPRanges {
		if r.RangeName == rangeName {
			return r.IPCIDRRange, nil
		}
	}
	return "", nil
}

// assignAliasIP read-modify-writes nic0's alias list, since gcloud --aliases is a full replacement.
func assignAliasIP(ctx context.Context, project, zone, instance, cidr string) error {
	logger := log.WithFunc("gke.assignAliasIP")

	current, err := describeNic0Aliases(ctx, project, zone, instance)
	if err != nil {
		return fmt.Errorf("describe nic0 aliases: %w", err)
	}

	for _, a := range current {
		if a.RangeName == aliasRangeName && a.IPCIDRRange == cidr {
			logger.Infof(ctx, "alias %s:%s already bound to %s; skipping gcloud update", aliasRangeName, cidr, instance)
			return nil
		}
	}

	merged := make([]string, 0, len(current)+1)
	for _, a := range current {
		if a.RangeName == aliasRangeName {
			logger.Infof(ctx, "replacing stale %s:%s on %s", aliasRangeName, a.IPCIDRRange, instance)
			continue
		}
		merged = append(merged, a.String())
	}
	merged = append(merged, fmt.Sprintf("%s:%s", aliasRangeName, cidr))

	if _, err := runGcloud(
		ctx,
		"compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", nic0Name,
		"--aliases", strings.Join(merged, ";"),
	); err != nil {
		return fmt.Errorf("assign alias: %w", err)
	}
	logger.Infof(ctx, "added alias %s:%s to %s; %d alias(es) total", aliasRangeName, cidr, instance, len(merged))
	return nil
}

// describeNic0Aliases returns nic0's alias IP ranges; an instance without nic0 is an error.
func describeNic0Aliases(ctx context.Context, project, zone, instance string) ([]aliasEntry, error) {
	out, err := runGcloud(
		ctx,
		"compute", "instances", "describe", instance,
		"--project", project, "--zone", zone,
		"--format", "json",
	)
	if err != nil {
		return nil, err
	}
	var desc struct {
		NetworkInterfaces []struct {
			Name          string       `json:"name"`
			AliasIPRanges []aliasEntry `json:"aliasIpRanges"`
		} `json:"networkInterfaces"`
	}
	if err := json.Unmarshal(out, &desc); err != nil {
		return nil, fmt.Errorf("parse instance describe: %w", err)
	}
	for _, ni := range desc.NetworkInterfaces {
		if ni.Name == nic0Name {
			return ni.AliasIPRanges, nil
		}
	}
	return nil, fmt.Errorf("%s not found on instance %s", nic0Name, instance)
}

// fixGuestAgentRoute removes the guest agent's local alias route, which black-holes VM return traffic, and reinstalls the fix via a boot cron.
func fixGuestAgentRoute(ctx context.Context, nic, cidr string) error {
	logger := log.WithFunc("gke.fixGuestAgentRoute")

	if err := delLocalAliasRoute(nic, cidr); err != nil {
		logger.Warnf(ctx, "del local route (ok if not found): %v", err)
	}

	cron := fmt.Sprintf("@reboot root ip route del local %s dev %s table local 2>/dev/null || true\n", cidr, nic)
	if err := os.WriteFile(cronFixFile, []byte(cron), filePerm); err != nil {
		return fmt.Errorf("write cron job %s: %w", cronFixFile, err)
	}
	logger.Infof(ctx, "installed alias route fix cron at %s", cronFixFile)
	return nil
}
