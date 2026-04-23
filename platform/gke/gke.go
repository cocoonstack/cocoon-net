// Package gke implements the CloudPlatform interface for Google Kubernetes Engine.
//
// TECH DEBT: this package currently drives GCE alias-IP management by shelling
// out to the `gcloud` CLI (see gcloud.go). This is an architectural bridge:
// the subprocess path is opaque to the Go runtime (no typed errors, no retries,
// no auth-refresh hooks) and relies on the operator having `gcloud`
// installed & authenticated.
//
// TODO: migrate to the official GCP Go SDK
// (cloud.google.com/go/compute/apiv1) for instances.UpdateNetworkInterface
// and subnetworks.Patch. This removes the `gcloud` binary dependency and
// surfaces typed error details (quotas, permission issues, propagation
// delays) directly to callers.
package gke

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metaBase       = "http://metadata.google.internal/computeMetadata/v1"
	aliasRangeName = "cocoon-pods"

	// DefaultNIC is the default primary NIC on GKE nodes.
	DefaultNIC = "ens4"

	detectionURL     = metaBase + "/instance/zone"
	detectionTimeout = 2 * time.Second
	metadataTimeout  = 5 * time.Second
)

var _ platform.CloudPlatform = (*GKE)(nil)

// GKE implements CloudPlatform for Google Kubernetes Engine.
type GKE struct{}

// Name returns the platform identifier.
func (g *GKE) Name() string { return platform.PlatformGKE }

// ProvisionNetwork configures a GCE alias IP range for the node.
func (g *GKE) ProvisionNetwork(ctx context.Context, cfg *platform.Config) (*platform.NetworkResult, error) {
	logger := log.WithFunc("gke.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = DefaultNIC
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
		Platform:   g.Name(),
		SubnetCIDR: cfg.SubnetCIDR,
		Gateway:    gateway,
		IPs:        ips,
		PrimaryNIC: primaryNIC,
	}, nil
}

// Status returns current alias IP status for this instance.
func (g *GKE) Status(_ context.Context) (*platform.PoolStatus, error) {
	return &platform.PoolStatus{}, nil
}
