package platform

import (
	"context"
	"fmt"
)

// CloudPlatform is the interface implemented by each cloud provider.
type CloudPlatform interface {
	// Name returns the platform identifier ("gke", "volcengine").
	Name() string
	// ProvisionNetwork provisions cloud networking resources for the node.
	// Returns the list of VPC-routable IPs available for VM DHCP.
	ProvisionNetwork(ctx context.Context, cfg *Config) (*NetworkResult, error)
	// Status returns current IP pool status.
	Status(ctx context.Context) (*PoolStatus, error)
	// Teardown removes cloud networking resources.
	Teardown(ctx context.Context) error
}

// Config holds the parameters for network provisioning.
type Config struct {
	NodeName   string   // virtual node name (e.g. "cocoon-pool")
	SubnetCIDR string   // desired VM subnet CIDR (e.g. "172.20.100.0/24")
	PoolSize   int      // desired number of IPs (default 140)
	Gateway    string   // cni0 gateway IP (e.g. "172.20.100.1")
	DNSServers []string // DNS for DHCP clients
	PrimaryNIC string   // host primary NIC (auto-detected if empty)
}

// NetworkResult is returned by ProvisionNetwork.
type NetworkResult struct {
	Platform   string // "gke" or "volcengine"
	SubnetCIDR string
	Gateway    string
	IPs        []string // VPC-routable IPs for DHCP pool
	PrimaryNIC string
}

// PoolStatus holds live status information from the cloud platform.
type PoolStatus struct {
	SubnetID string
	ENIIDs   []string
	IPs      []string
}

// New returns a CloudPlatform by name without auto-detecting.
func New(name string) (CloudPlatform, error) {
	switch name {
	case "gke":
		return &GKEPlatform{}, nil
	case "volcengine":
		return &VolcenginePlatform{}, nil
	default:
		return nil, fmt.Errorf("unknown platform: %s (valid: gke, volcengine)", name)
	}
}
