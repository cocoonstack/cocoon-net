package gke

import (
	"cmp"
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

func (g *GKE) Adopt(ctx context.Context, cfg *platform.Config) error {
	return fixGuestAgentRoute(ctx, cmp.Or(cfg.PrimaryNIC, platform.DefaultNIC(g.Name())), cfg.SubnetCIDR)
}
