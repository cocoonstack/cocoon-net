package volcengine

import (
	"context"
	"fmt"

	"github.com/cocoonstack/cocoon-net/platform"
)

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
	statuses := make([]platform.ENIStatus, 0, len(enis))
	for _, e := range enis {
		eniIDs = append(eniIDs, e.NetworkInterfaceID)
		var own []string
		for _, pip := range e.PrivateIPSets.PrivateIPSet {
			if !pip.Primary {
				own = append(own, pip.PrivateIPAddress)
			}
		}
		statuses = append(statuses, platform.ENIStatus{ID: e.NetworkInterfaceID, MAC: e.MacAddress, IPs: own})
		ips = append(ips, own...)
	}

	return &platform.PoolStatus{
		ENIIDs: eniIDs,
		IPs:    ips,
		ENIs:   statuses,
	}, nil
}
