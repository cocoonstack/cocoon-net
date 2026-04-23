package volcengine

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

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
