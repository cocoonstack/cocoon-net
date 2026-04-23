package platform

import (
	"context"
	"fmt"
)

const (
	PlatformGKE        = "gke"
	PlatformVolcengine = "volcengine"

	// volcengineSecondaryNICCount is the fixed number of secondary ENIs per
	// Volcengine node (eth1..eth7). This matches the platform's default ENI
	// quota per instance.
	volcengineSecondaryNICCount = 7
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
		nics := make([]string, volcengineSecondaryNICCount)
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
	// Teardown removes cloud networking resources belonging to this node.
	// The caller passes in persisted state (alias range name, subnet CIDR,
	// etc.) so platforms can remove exactly what they created without
	// touching resources shared across nodes.
	Teardown(ctx context.Context, cfg *TeardownConfig) error
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
	Platform       string
	SubnetCIDR     string
	Gateway        string
	PrimaryNIC     string
	SecondaryNICs  []string // Volcengine: eth1..eth7; GKE: nil
	IPs            []string
	AliasRangeName string // GKE: the GCE secondary range the alias came from; empty on other platforms
}

// TeardownConfig is the persisted-state snapshot Teardown needs to undo
// exactly what this node claimed at init/adopt time.
type TeardownConfig struct {
	// AliasRangeName is the GCE secondary range the node's alias was bound
	// from (GKE). Empty means "use platform default" — set for state written
	// before this field existed, or for adopted nodes whose alias was
	// provisioned manually.
	AliasRangeName string
	// SubnetCIDR is the per-node alias CIDR to remove (GKE) or, for
	// Volcengine, informational only (teardown walks attached ENIs).
	SubnetCIDR string
}

// PoolStatus holds live status information from the cloud platform.
type PoolStatus struct {
	SubnetID string
	ENIIDs   []string
	IPs      []string
}
