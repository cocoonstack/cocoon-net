package gke

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

func (g *GKE) Adopt(ctx context.Context, cfg *platform.Config) error {
	return fixGuestAgentRoute(ctx, cfg.PrimaryNIC, cfg.SubnetCIDR)
}
