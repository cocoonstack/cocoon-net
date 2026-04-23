package gke

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Teardown removes the alias IP range from the instance.
//
// Precision pass (PR3) will filter current nic0 aliases and remove only the
// one owned by this node; today the behavior is still the blunt `--aliases ""`
// sweep, but the signature is already widened so callers persist the state
// needed for precision removal.
func (g *GKE) Teardown(ctx context.Context, _ *platform.TeardownConfig) error {
	logger := log.WithFunc("gke.Teardown")
	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	_, err = gcloudRun(ctx,
		"compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", "nic0",
		"--aliases", "",
	)
	if err != nil {
		return fmt.Errorf("remove alias IP: %w", err)
	}
	logger.Infof(ctx, "alias IP removed from %s", instance)
	return nil
}
