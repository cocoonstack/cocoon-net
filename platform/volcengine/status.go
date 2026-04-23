package volcengine

import (
	"context"
	"fmt"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Status returns the current ENI and IP status.
func (v *Volcengine) Status(ctx context.Context) (*platform.PoolStatus, error) {
	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}

	enis, err := listENIs(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list ENIs: %w", err)
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
