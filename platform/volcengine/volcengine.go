// Package volcengine implements the CloudPlatform interface for Volcengine.
//
// TECH DEBT: this package drives the Volcengine cloud API by shelling out to
// the `ve` CLI (see ve.go). This is an architectural bridge: the subprocess
// path is opaque to the Go runtime (no typed errors, no retries, no request
// tracing) and depends on the operator having the `ve` binary installed.
//
// TODO: migrate to the official Volcengine Go SDK
// (github.com/volcengine/volcengine-go-sdk) for all vpc/DescribeSecurityGroups,
// DescribeSubnets, CreateSubnet, {Create,Attach,Detach,Delete}NetworkInterface,
// AssignPrivateIpAddresses, DescribeNetworkInterfaces calls. This removes the
// `ve` binary dependency and gives us typed responses and structured errors.
package volcengine

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metadataBase = "http://100.96.0.96/latest/meta-data"
	defaultNIC   = "eth0"
	enisPerNode  = 7
	ipsPerENI    = 20

	eniTypePrimary = "primary"
)

var _ platform.CloudPlatform = (*Volcengine)(nil)

// Volcengine implements CloudPlatform for Volcengine.
//
// Credentials and region are resolved once during New() — do not rely on
// hidden per-call env initialisation.
type Volcengine struct {
	env *envConfig
}

// New constructs a Volcengine platform handle, loading credentials from
// env vars or ~/.volcengine/config.json exactly once.
func New(ctx context.Context) (*Volcengine, error) {
	env, err := loadEnv(ctx)
	if err != nil {
		return nil, fmt.Errorf("load volcengine env: %w", err)
	}
	return &Volcengine{env: env}, nil
}

// Name returns the platform identifier.
func (v *Volcengine) Name() string { return platform.PlatformVolcengine }

// Status returns the current ENI and IP status.
func (v *Volcengine) Status(ctx context.Context) (*platform.PoolStatus, error) {
	logger := log.WithFunc("volcengine.Status")

	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}

	enis, err := listENIs(ctx, instanceID)
	if err != nil {
		logger.Warnf(ctx, "list ENIs: %v", err)
		return &platform.PoolStatus{}, nil
	}

	var eniIDs, ips []string
	for _, e := range enis {
		eniIDs = append(eniIDs, e.NetworkInterfaceID)
		for _, pip := range e.PrivateIPSets.PrivateIPSet {
			if !pip.Primary {
				ips = append(ips, pip.PrivateIPAddress)
			}
		}
	}

	return &platform.PoolStatus{
		ENIIDs: eniIDs,
		IPs:    ips,
	}, nil
}
