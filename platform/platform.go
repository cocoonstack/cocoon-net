// Package platform abstracts cloud-specific network provisioning behind CloudPlatform, with auto-detection and per-cloud implementations.
package platform

import (
	"context"
	"fmt"
)

const (
	PlatformGKE        = "gke"
	PlatformVolcengine = "volcengine"

	// volcengineSecondaryNICCount matches Volcengine's default ENI quota per instance (eth1..eth7).
	volcengineSecondaryNICCount = 7
)

// CloudPlatform is the interface implemented by each cloud provider.
type CloudPlatform interface {
	// Name returns the platform identifier ("gke", "volcengine").
	Name() string
	// ProvisionNetwork provisions cloud networking resources for the node.
	ProvisionNetwork(ctx context.Context, cfg *Config) (*NetworkResult, error)
	// Status returns current IP pool status.
	Status(ctx context.Context) (*PoolStatus, error)
	// Teardown removes the cloud resources this node claimed, using the persisted state in cfg.
	Teardown(ctx context.Context, cfg *TeardownConfig) error
	// Adopt applies the host-side fixes a hand-provisioned node needs; it makes no cloud API calls.
	Adopt(ctx context.Context, cfg *Config) error
}

// Config holds the parameters for network provisioning.
type Config struct {
	NodeName   string
	SubnetCIDR string
	PoolSize   int
	Gateway    string
	PrimaryNIC string
}

// NetworkResult is returned by ProvisionNetwork.
type NetworkResult struct {
	Platform       string
	SubnetCIDR     string
	Gateway        string
	PrimaryNIC     string
	SecondaryNICs  []string // Volcengine: eth1..eth7; GKE: nil
	ENIIDs         []string
	IPs            []string
	AliasRangeName string // GKE: the GCE secondary range the alias came from; empty on other platforms
}

// TeardownConfig is the persisted state Teardown needs to undo what this node claimed.
type TeardownConfig struct {
	ENIIDs []string
	// AliasRangeName is the GCE secondary range the alias was bound from; empty means the package default.
	AliasRangeName string
	// SubnetCIDR is the per-node alias CIDR to remove on GKE; informational on Volcengine.
	SubnetCIDR string
}

// PoolStatus holds live status information from the cloud platform.
type PoolStatus struct {
	SubnetID string
	ENIIDs   []string
	IPs      []string
	// AliasRanges lists NAME:CIDR entries bound to the primary NIC (GKE only).
	AliasRanges []string
}

// DefaultNIC returns the default primary NIC for a given platform.
func DefaultNIC(platformName string) string {
	switch platformName {
	case PlatformVolcengine:
		return "eth0"
	default:
		return "ens4"
	}
}

// DefaultSecondaryNICs returns eth1..eth7 for Volcengine; nil elsewhere.
func DefaultSecondaryNICs(platformName string) []string {
	if platformName == PlatformVolcengine {
		return SecondaryNICNames(volcengineSecondaryNICCount)
	}
	return nil
}

// SecondaryNICNames returns eth1..ethN, the kernel names of N attached ENIs.
func SecondaryNICNames(n int) []string {
	nics := make([]string, n)
	for i := range nics {
		nics[i] = fmt.Sprintf("eth%d", i+1)
	}
	return nics
}
