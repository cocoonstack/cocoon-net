package gke

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Status returns current alias IP status for this instance.
func (g *GKE) Status(_ context.Context) (*platform.PoolStatus, error) {
	return &platform.PoolStatus{}, nil
}
