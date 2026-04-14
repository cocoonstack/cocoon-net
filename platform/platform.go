package platform

import (
	"context"
	"fmt"
)

const (
	PlatformGKE        = "gke"
	PlatformVolcengine = "volcengine"
)

// DefaultNIC returns the default primary NIC for a given platform.
func DefaultNIC(platformName string) string {
	switch platformName {
	case PlatformVolcengine:
		return "eth0"
	default:
		return "ens4"
	}
}

// DefaultSecondaryNICs returns the expected secondary NIC names for a platform.
// GKE has no secondary NICs; Volcengine uses eth1..eth7 for ENIs.
func DefaultSecondaryNICs(platformName string) []string {
	switch platformName {
	case PlatformVolcengine:
		nics := make([]string, 7) //nolint:mnd
		for i := range nics {
			nics[i] = fmt.Sprintf("eth%d", i+1)
		}
		return nics
	default:
		return nil
	}
}

// CloudPlatform is the interface implemented by each cloud provider.
type CloudPlatform interface {
	// Name returns the platform identifier ("gke", "volcengine").
	Name() string
	// ProvisionNetwork provisions cloud networking resources for the node.
	ProvisionNetwork(ctx context.Context, cfg *Config) (*NetworkResult, error)
	// Status returns current IP pool status.
	Status(ctx context.Context) (*PoolStatus, error)
	// Teardown removes cloud networking resources.
	Teardown(ctx context.Context) error
}

// Config holds the parameters for network provisioning.
type Config struct {
	NodeName   string
	SubnetCIDR string
	PoolSize   int
	Gateway    string
	DNSServers []string
	PrimaryNIC string
}

// NetworkResult is returned by ProvisionNetwork.
type NetworkResult struct {
	Platform      string
	SubnetCIDR    string
	Gateway       string
	PrimaryNIC    string
	SecondaryNICs []string // Volcengine: eth1..eth7; GKE: nil
	IPs           []string
}

// PoolStatus holds live status information from the cloud platform.
type PoolStatus struct {
	SubnetID string
	ENIIDs   []string
	IPs      []string
}
