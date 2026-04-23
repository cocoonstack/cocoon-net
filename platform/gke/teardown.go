package gke

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Teardown removes this node's alias IP range from nic0 while preserving any
// other alias entries present on the interface and leaving the shared
// `cocoon-pods` secondary range on the GCE subnet intact. That secondary
// range is operator-owned and shared across nodes — see docs/gke.md.
//
// The cron entry installed by fixGuestAgentRoute is also removed.
func (g *GKE) Teardown(ctx context.Context, cfg *platform.TeardownConfig) error {
	logger := log.WithFunc("gke.Teardown")

	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	// Fall back to the package default when the caller's state predates the
	// AliasRangeName field (e.g. state written by an older cocoon-net) or was
	// created via `adopt`, where the name was not captured.
	rangeName := cfg.AliasRangeName
	if rangeName == "" {
		rangeName = aliasRangeName
	}

	current, err := describeNic0Aliases(ctx, project, zone, instance)
	if err != nil {
		return fmt.Errorf("describe nic0 aliases: %w", err)
	}

	kept := make([]string, 0, len(current))
	removed := false
	for _, a := range current {
		if a.RangeName == rangeName && a.IPCIDRRange == cfg.SubnetCIDR {
			removed = true
			continue
		}
		kept = append(kept, formatAlias(a))
	}
	if !removed {
		logger.Warnf(ctx, "alias %s:%s not present on nic0 of %s; nothing to remove", rangeName, cfg.SubnetCIDR, instance)
	}

	if _, err := gcloudRun(ctx,
		"compute", "instances", "network-interfaces", "update", instance,
		"--project", project, "--zone", zone,
		"--network-interface", "nic0",
		"--aliases", strings.Join(kept, ";"),
	); err != nil {
		return fmt.Errorf("update aliases on %s: %w", instance, err)
	}

	if err := os.Remove(cronFixFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Warnf(ctx, "remove cron %s: %v", cronFixFile, err)
	}

	logger.Infof(ctx, "removed alias %s:%s from %s (%d kept); shared secondary range %s preserved",
		rangeName, cfg.SubnetCIDR, instance, len(kept), rangeName)
	return nil
}

// formatAlias renders one alias entry in the form gcloud expects for
// `--aliases`. Entries without a range name come from the subnet's primary
// range and are passed as CIDR-only.
func formatAlias(a aliasEntry) string {
	if a.RangeName == "" {
		return a.IPCIDRRange
	}
	return fmt.Sprintf("%s:%s", a.RangeName, a.IPCIDRRange)
}
