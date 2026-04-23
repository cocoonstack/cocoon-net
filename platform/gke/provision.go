package gke

import (
	"context"
	"fmt"
	"strings"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// ProvisionNetwork configures a GCE alias IP range for the node.
func (g *GKE) ProvisionNetwork(ctx context.Context, cfg *platform.Config) (*platform.NetworkResult, error) {
	logger := log.WithFunc("gke.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = platform.DefaultNIC(g.Name())
	}

	instance, zone, project, subnet, err := fetchMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch gce metadata: %w", err)
	}
	logger.Infof(ctx, "instance=%s zone=%s project=%s subnet=%s", instance, zone, project, subnet)

	region := zone[:strings.LastIndex(zone, "-")]

	if err = ensureSecondaryRange(ctx, project, region, subnet, cfg.SubnetCIDR); err != nil {
		return nil, fmt.Errorf("ensure secondary range: %w", err)
	}

	if err = assignAliasIP(ctx, project, zone, instance, cfg.SubnetCIDR); err != nil {
		return nil, fmt.Errorf("assign alias IP: %w", err)
	}

	if err = fixGuestAgentRoute(ctx, primaryNIC, cfg.SubnetCIDR); err != nil {
		logger.Warnf(ctx, "fix guest agent route: %v", err)
	}

	gateway := cfg.Gateway
	if gateway == "" {
		gateway, err = platform.FirstHostIP(cfg.SubnetCIDR)
		if err != nil {
			return nil, fmt.Errorf("compute gateway: %w", err)
		}
	}

	ips, err := platform.SubnetIPs(cfg.SubnetCIDR, gateway, cfg.PoolSize)
	if err != nil {
		return nil, fmt.Errorf("compute ip list: %w", err)
	}

	return &platform.NetworkResult{
		Platform:       g.Name(),
		SubnetCIDR:     cfg.SubnetCIDR,
		Gateway:        gateway,
		IPs:            ips,
		PrimaryNIC:     primaryNIC,
		AliasRangeName: aliasRangeName,
	}, nil
}
