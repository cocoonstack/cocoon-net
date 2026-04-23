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

// Teardown removes this node's alias from nic0 and preserves the shared
// `cocoon-pods` secondary range (operator-owned; see docs/gke.md). Other
// alias entries on nic0 are kept; the fixGuestAgentRoute cron is removed
// best-effort.
func (g *GKE) Teardown(ctx context.Context, cfg *platform.TeardownConfig) error {
	logger := log.WithFunc("gke.Teardown")

	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	// Fallback for state written before AliasRangeName existed and for adopted nodes.
	rangeName := cmp.Or(cfg.AliasRangeName, aliasRangeName)

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
		if _, err := gcloudRun(ctx,
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
