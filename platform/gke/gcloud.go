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

// gcloudRun executes the `gcloud` CLI. Every invocation is a tech-debt
// hotspot documented at package level — see gke.go.
func gcloudRun(ctx context.Context, args ...string) ([]byte, error) {
	return platform.RunSubprocess(ctx, "gcloud", args...)
}

// ensureSecondaryRange guarantees that the named secondary range on the GCE
// subnet covers cidr. An existing range that is a superset is reused; one
// that does not cover cidr produces an up-front error; a missing range is
// created at cidr (single-node cold-start path; see docs/gke.md).
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
	if _, err := gcloudRun(
		ctx,
		"compute", "networks", "subnets", "update", subnet,
		"--project", project, "--region", region,
		fmt.Sprintf("--add-secondary-ranges=%s=%s", aliasRangeName, cidr),
	); err != nil {
		return fmt.Errorf("create secondary range %s=%s: %w", aliasRangeName, cidr, err)
	}
	return nil
}

// describeSecondaryRange returns the CIDR of the named secondary range on the
// GCE subnet, or "" if no range with that name exists.
func describeSecondaryRange(ctx context.Context, project, region, subnet, rangeName string) (string, error) {
	out, err := gcloudRun(
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

// assignAliasIP merges our alias into nic0 via read-modify-write
// (gcloud --aliases is a full replacement). No-op if our exact entry
// is already present; stale entries under our range name are replaced.
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

	if _, err := gcloudRun(
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

// describeNic0Aliases returns the alias IP ranges currently bound to nic0
// on the given instance; errors if nic0 is absent from the describe output.
func describeNic0Aliases(ctx context.Context, project, zone, instance string) ([]aliasEntry, error) {
	out, err := gcloudRun(
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

// fixGuestAgentRoute deletes the local route the GCE guest agent auto-installs
// for alias ranges (it would black-hole VM return traffic) and persists a boot
// cron to re-apply it — the cron shells out to `ip route` since it runs before the daemon.
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
