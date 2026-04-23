package volcengine

import (
	"context"
	"fmt"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

// ProvisionNetwork provisions Volcengine networking resources.
func (v *Volcengine) ProvisionNetwork(ctx context.Context, cfg *platform.Config) (*platform.NetworkResult, error) {
	logger := log.WithFunc("volcengine.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = platform.DefaultNIC(v.Name())
	}

	vpcID, err := fetchMeta(ctx, "/vpc-id")
	if err != nil {
		return nil, fmt.Errorf("get vpc id: %w", err)
	}

	sgID, err := getSecurityGroupID(ctx, vpcID)
	if err != nil {
		return nil, fmt.Errorf("get security group id: %w", err)
	}

	subnetID, err := ensureSubnet(ctx, vpcID, cfg.SubnetCIDR, cfg.NodeName)
	if err != nil {
		return nil, fmt.Errorf("ensure subnet: %w", err)
	}
	logger.Infof(ctx, "subnet %s (id=%s)", cfg.SubnetCIDR, subnetID)

	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}
	logger.Infof(ctx, "instance id: %s", instanceID)

	eniIDs, err := createAndAttachENIs(ctx, subnetID, sgID, instanceID, cfg.NodeName, enisPerNode)
	if err != nil {
		return nil, fmt.Errorf("create/attach ENIs: %w", err)
	}
	logger.Infof(ctx, "attached %d ENIs", len(eniIDs))

	var allIPs []string
	for _, eniID := range eniIDs {
		ips, assignErr := assignSecondaryIPs(ctx, eniID, ipsPerENI)
		if assignErr != nil {
			// Partial-failure: this ENI is abandoned but other ENIs may still
			// yield a usable pool. Log the failure and bail out below only if
			// every ENI failed.
			logger.Errorf(ctx, assignErr, "assign secondary IPs to %s", eniID)
			continue
		}
		allIPs = append(allIPs, ips...)
	}
	if len(allIPs) == 0 {
		return nil, fmt.Errorf("no secondary IPs assigned across %d ENIs", len(eniIDs))
	}
	logger.Infof(ctx, "assigned %d secondary IPs", len(allIPs))

	// Bring up secondary interfaces via netlink. A failure here means the
	// secondary NIC is down and its assigned IPs are unreachable, so the
	// pool would hand out unusable addresses — fail fast instead.
	secondaryNICs := platform.DefaultSecondaryNICs(v.Name())
	for _, iface := range secondaryNICs {
		if linkErr := bringLinkUp(iface); linkErr != nil {
			return nil, fmt.Errorf("bring up %s: %w", iface, linkErr)
		}
	}

	gateway := cfg.Gateway
	if gateway == "" {
		gateway, err = platform.FirstHostIP(cfg.SubnetCIDR)
		if err != nil {
			return nil, fmt.Errorf("compute gateway: %w", err)
		}
	}

	platform.SortIPs(allIPs)

	return &platform.NetworkResult{
		Platform:      v.Name(),
		SubnetCIDR:    cfg.SubnetCIDR,
		Gateway:       gateway,
		PrimaryNIC:    primaryNIC,
		SecondaryNICs: secondaryNICs,
		IPs:           allIPs,
	}, nil
}
