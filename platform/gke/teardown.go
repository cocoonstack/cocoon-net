package gke

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Only this node's alias is removed; the shared secondary range is
// operator-owned and preserved (see docs/gke.md).
func (g *GKE) Teardown(ctx context.Context, cfg *platform.TeardownConfig) error {
	logger := log.WithFunc("gke.Teardown")

	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	rangeName := resolveAliasRangeName(cfg.AliasRangeName)

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
		kept = append(kept, a.String())
	}

	if !removed {
		logger.Warnf(ctx, "alias %s:%s not present on nic0 of %s; skipping gcloud update", rangeName, cfg.SubnetCIDR, instance)
	} else {
		if _, err := gcloudRun(
			ctx,
			"compute", "instances", "network-interfaces", "update", instance,
			"--project", project, "--zone", zone,
			"--network-interface", nic0Name,
			"--aliases", strings.Join(kept, ";"),
		); err != nil {
			return fmt.Errorf("update aliases on %s: %w", instance, err)
		}
		logger.Infof(ctx, "removed alias %s:%s from %s; %d alias(es) remain", rangeName, cfg.SubnetCIDR, instance, len(kept))
	}

	if err := os.Remove(cronFixFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Warnf(ctx, "remove cron %s: %v", cronFixFile, err)
	}

	return nil
}

// resolveAliasRangeName falls back to the package default when the
// persisted range name is empty — state written before AliasRangeName
// existed, or a node adopted without an explicit range.
func resolveAliasRangeName(rangeName string) string {
	return cmp.Or(rangeName, aliasRangeName)
}
