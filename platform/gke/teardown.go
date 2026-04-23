package gke

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"
)

// Teardown removes the alias IP range from the instance.
func (g *GKE) Teardown(ctx context.Context) error {
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
