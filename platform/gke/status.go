package gke

import (
	"context"
	"fmt"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Status reports the alias IP ranges currently bound to nic0, giving
// operators a view of cloud-side reality to compare against pool.json.
func (g *GKE) Status(ctx context.Context) (*platform.PoolStatus, error) {
	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch gce metadata: %w", err)
	}

	aliases, err := describeNic0Aliases(ctx, project, zone, instance)
	if err != nil {
		return nil, fmt.Errorf("describe nic0 aliases: %w", err)
	}

	ranges := make([]string, 0, len(aliases))
	for _, a := range aliases {
		ranges = append(ranges, a.String())
	}
	return &platform.PoolStatus{AliasRanges: ranges}, nil
}
