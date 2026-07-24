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

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metadataBase = "http://100.96.0.96/latest/meta-data"
	enisPerNode  = 7
	ipsPerENI    = 20

	eniTypePrimary = "primary"
)

var _ platform.CloudPlatform = (*Volcengine)(nil)

// Volcengine implements CloudPlatform; the struct is empty because credentials live in the env of the `ve` child binary.
type Volcengine struct{}

// New loads credentials from env or ~/.volcengine/config.json exactly once.
func New(ctx context.Context) (*Volcengine, error) {
	if err := loadEnv(ctx); err != nil {
		return nil, fmt.Errorf("load volcengine env: %w", err)
	}
	return &Volcengine{}, nil
}

func (v *Volcengine) Name() string { return platform.PlatformVolcengine }
