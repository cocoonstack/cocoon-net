package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/projecteru2/core/log"
)

const (
	vePrimaryNICName = "eth0"
	veENIsPerNode    = 7
	veIPsPerENI      = 20
)

// VolcenginePlatform implements CloudPlatform for Volcengine (火山引擎).
type VolcenginePlatform struct{}

// Name returns the platform identifier.
func (v *VolcenginePlatform) Name() string { return "volcengine" }

// ProvisionNetwork provisions Volcengine networking:
//  1. Detect/create VM subnet.
//  2. Create ENIs in the subnet.
//  3. Attach ENIs to the instance.
//  4. Assign secondary IPs (20 per ENI).
//  5. Bring up ethX interfaces.
//  6. Return secondary IPs.
func (v *VolcenginePlatform) ProvisionNetwork(ctx context.Context, cfg *Config) (*NetworkResult, error) {
	logger := log.WithFunc("platform.volcengine.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = vePrimaryNICName
	}

	// Set credentials from config file / env.
	if err := veSetupEnv(ctx); err != nil {
		return nil, fmt.Errorf("setup volcengine credentials: %w", err)
	}

	vpcID, err := veGetVPCID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get vpc id: %w", err)
	}

	sgID, err := veGetSecurityGroupID(ctx, vpcID)
	if err != nil {
		return nil, fmt.Errorf("get security group id: %w", err)
	}

	// Ensure VM subnet exists.
	subnetID, err := veEnsureSubnet(ctx, vpcID, cfg.SubnetCIDR, cfg.NodeName)
	if err != nil {
		return nil, fmt.Errorf("ensure subnet: %w", err)
	}
	logger.Infof(ctx, "subnet %s (id=%s)", cfg.SubnetCIDR, subnetID)

	instanceID, err := veGetInstanceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}
	logger.Infof(ctx, "instance id: %s", instanceID)

	// Create ENIs and collect secondary IPs.
	eniIDs, err := veCreateAndAttachENIs(ctx, subnetID, sgID, instanceID, cfg.NodeName, veENIsPerNode)
	if err != nil {
		return nil, fmt.Errorf("create/attach ENIs: %w", err)
	}
	logger.Infof(ctx, "attached %d ENIs", len(eniIDs))

	var allIPs []string
	for _, eniID := range eniIDs {
		ips, err := veAssignSecondaryIPs(ctx, eniID, veIPsPerENI)
		if err != nil {
			logger.Warnf(ctx, "assign secondary IPs to %s: %v", eniID, err)
			continue
		}
		allIPs = append(allIPs, ips...)
	}
	logger.Infof(ctx, "assigned %d secondary IPs", len(allIPs))

	// Bring up secondary interfaces.
	for i := 1; i <= veENIsPerNode; i++ {
		iface := fmt.Sprintf("eth%d", i)
		//nolint:gosec // ip args from trusted NIC name
		cmd := exec.CommandContext(ctx, "ip", "link", "set", iface, "up")
		out, err := cmd.CombinedOutput()
		if err != nil {
			logger.Warnf(ctx, "bring up %s: %v: %s", iface, err, out)
		}
	}

	gateway := cfg.Gateway
	if gateway == "" {
		var err error
		gateway, err = firstHostIP(cfg.SubnetCIDR)
		if err != nil {
			return nil, fmt.Errorf("compute gateway: %w", err)
		}
	}

	sort.Slice(allIPs, func(i, j int) bool {
		return ipLess(allIPs[i], allIPs[j])
	})

	return &NetworkResult{
		Platform:   v.Name(),
		SubnetCIDR: cfg.SubnetCIDR,
		Gateway:    gateway,
		IPs:        allIPs,
		PrimaryNIC: primaryNIC,
	}, nil
}

// Status returns the current ENI and IP status.
func (v *VolcenginePlatform) Status(ctx context.Context) (*PoolStatus, error) {
	logger := log.WithFunc("platform.volcengine.Status")

	if err := veSetupEnv(ctx); err != nil {
		return nil, fmt.Errorf("setup credentials: %w", err)
	}

	instanceID, err := veGetInstanceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get instance id: %w", err)
	}

	enis, err := veListENIs(ctx, instanceID)
	if err != nil {
		logger.Warnf(ctx, "list ENIs: %v", err)
		return &PoolStatus{}, nil
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

	return &PoolStatus{
		ENIIDs: eniIDs,
		IPs:    ips,
	}, nil
}

// Teardown detaches and deletes all secondary ENIs for this instance.
func (v *VolcenginePlatform) Teardown(ctx context.Context) error {
	logger := log.WithFunc("platform.volcengine.Teardown")

	if err := veSetupEnv(ctx); err != nil {
		return fmt.Errorf("setup credentials: %w", err)
	}

	instanceID, err := veGetInstanceID(ctx)
	if err != nil {
		return fmt.Errorf("get instance id: %w", err)
	}

	enis, err := veListENIs(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("list ENIs: %w", err)
	}

	for _, eni := range enis {
		if eni.Type == "primary" {
			continue
		}

		//nolint:gosec // ve args from trusted ENI metadata
		detachCmd := exec.CommandContext(ctx, "ve", "vpc", "DetachNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
			"--InstanceId", instanceID,
		)
		out, err := detachCmd.CombinedOutput()
		if err != nil {
			logger.Warnf(ctx, "detach ENI %s: %v: %s", eni.NetworkInterfaceID, err, out)
		}

		//nolint:gosec // ve args from trusted ENI metadata
		delCmd := exec.CommandContext(ctx, "ve", "vpc", "DeleteNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
		)
		out, err = delCmd.CombinedOutput()
		if err != nil {
			logger.Warnf(ctx, "delete ENI %s: %v: %s", eni.NetworkInterfaceID, err, out)
		} else {
			logger.Infof(ctx, "deleted ENI %s", eni.NetworkInterfaceID)
		}
	}
	return nil
}

// --- Volcengine API helpers via `ve` CLI ---

type veNetworkInterface struct {
	NetworkInterfaceID string `json:"NetworkInterfaceId"`
	Type               string `json:"Type"`
	PrivateIPSets      struct {
		PrivateIPSet []struct {
			Primary          bool   `json:"Primary"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
		} `json:"PrivateIpSet"`
	} `json:"PrivateIpSets"`
}

func veRun(ctx context.Context, args ...string) ([]byte, error) {
	//nolint:gosec // args come from internal constants and metadata, not user input
	cmd := exec.CommandContext(ctx, "ve", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ve %s: %w: %s", strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}

func veGetInstanceID(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "bash", "-c",
		`curl -sf http://100.96.0.96/latest/meta-data/instance-id`,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fetch instance-id metadata: %w: %s", err, out)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("instance-id metadata returned empty")
	}
	return id, nil
}

func veGetVPCID(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "bash", "-c",
		`curl -sf http://100.96.0.96/latest/meta-data/vpc-id`,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fetch vpc-id metadata: %w: %s", err, out)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("vpc-id metadata returned empty")
	}
	return id, nil
}

func veGetSecurityGroupID(ctx context.Context, vpcID string) (string, error) {
	out, err := veRun(ctx, "vpc", "DescribeSecurityGroups",
		"--VpcId", vpcID, "--PageSize", "1",
	)
	if err != nil {
		return "", fmt.Errorf("describe security groups: %w", err)
	}
	var resp struct {
		Result struct {
			SecurityGroups []struct {
				SecurityGroupID string `json:"SecurityGroupId"`
			} `json:"SecurityGroups"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse security groups: %w", err)
	}
	if len(resp.Result.SecurityGroups) == 0 {
		return "", fmt.Errorf("no security groups found in VPC %s", vpcID)
	}
	return resp.Result.SecurityGroups[0].SecurityGroupID, nil
}

func veEnsureSubnet(ctx context.Context, vpcID, cidr, name string) (string, error) {
	// Check for existing subnet.
	out, err := veRun(ctx, "vpc", "DescribeSubnets",
		"--VpcId", vpcID, "--PageSize", "100",
	)
	if err == nil {
		var resp struct {
			Result struct {
				Subnets []struct {
					SubnetID  string `json:"SubnetId"`
					CIDRBlock string `json:"CidrBlock"`
				} `json:"Subnets"`
			} `json:"Result"`
		}
		if json.Unmarshal(out, &resp) == nil {
			for _, s := range resp.Result.Subnets {
				if s.CIDRBlock == cidr {
					return s.SubnetID, nil
				}
			}
		}
	}

	// Get zone from metadata.
	zoneOut, err := exec.CommandContext(ctx, "bash", "-c",
		`curl -sf http://100.96.0.96/latest/meta-data/placement/availability-zone`,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fetch zone metadata: %w", err)
	}
	zone := strings.TrimSpace(string(zoneOut))

	// Create subnet.
	createOut, err := veRun(ctx, "vpc", "CreateSubnet",
		"--VpcId", vpcID,
		"--CidrBlock", cidr,
		"--ZoneId", zone,
		"--SubnetName", name+"-vms",
	)
	if err != nil {
		return "", fmt.Errorf("create subnet: %w", err)
	}
	var resp struct {
		Result struct {
			SubnetID string `json:"SubnetId"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(createOut, &resp); err != nil {
		return "", fmt.Errorf("parse create subnet response: %w", err)
	}
	return resp.Result.SubnetID, nil
}

func veCreateAndAttachENIs(ctx context.Context, subnetID, sgID, instanceID, prefix string, count int) ([]string, error) {
	logger := log.WithFunc("platform.volcengine.veCreateAndAttachENIs")
	var eniIDs []string

	for i := 1; i <= count; i++ {
		out, err := veRun(ctx, "vpc", "CreateNetworkInterface",
			"--SubnetId", subnetID,
			"--SecurityGroupIds.1", sgID,
			"--NetworkInterfaceName", fmt.Sprintf("%s-eni-%d", prefix, i),
		)
		if err != nil {
			return nil, fmt.Errorf("create ENI %d: %w", i, err)
		}
		var resp struct {
			Result struct {
				NetworkInterfaceID string `json:"NetworkInterfaceId"`
			} `json:"Result"`
		}
		if unmarshalErr := json.Unmarshal(out, &resp); unmarshalErr != nil {
			return nil, fmt.Errorf("parse create ENI %d response: %w", i, unmarshalErr)
		}
		eniID := resp.Result.NetworkInterfaceID

		// Attach to instance.
		//nolint:gosec // sleep is needed for API propagation
		_ = exec.CommandContext(ctx, "sleep", "2").Run()
		_, err = veRun(ctx, "vpc", "AttachNetworkInterface",
			"--NetworkInterfaceId", eniID,
			"--InstanceId", instanceID,
		)
		if err != nil {
			logger.Warnf(ctx, "attach ENI %s: %v", eniID, err)
		}
		//nolint:gosec // sleep is needed for API propagation
		_ = exec.CommandContext(ctx, "sleep", "4").Run()

		eniIDs = append(eniIDs, eniID)
		logger.Infof(ctx, "created and attached ENI %s (%d/%d)", eniID, i, count)
	}
	return eniIDs, nil
}

func veAssignSecondaryIPs(ctx context.Context, eniID string, count int) ([]string, error) {
	out, err := veRun(ctx, "vpc", "AssignPrivateIpAddresses",
		"--NetworkInterfaceId", eniID,
		"--SecondaryPrivateIpAddressCount", fmt.Sprintf("%d", count),
	)
	if err != nil {
		return nil, fmt.Errorf("assign secondary IPs to %s: %w", eniID, err)
	}

	var resp struct {
		Result struct {
			PrivateIPSet []string `json:"PrivateIpSet"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse assign IPs response: %w", err)
	}
	return resp.Result.PrivateIPSet, nil
}

func veListENIs(ctx context.Context, instanceID string) ([]veNetworkInterface, error) {
	out, err := veRun(ctx, "vpc", "DescribeNetworkInterfaces",
		"--InstanceId", instanceID,
		"--PageSize", "100",
	)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			NetworkInterfaceSets []veNetworkInterface `json:"NetworkInterfaceSets"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse ENI list: %w", err)
	}
	return resp.Result.NetworkInterfaceSets, nil
}

// veSetupEnv reads Volcengine credentials from config file or env vars.
func veSetupEnv(_ context.Context) error {
	// If already set via environment, trust them.
	if os.Getenv("VOLCENGINE_ACCESS_KEY_ID") != "" {
		return nil
	}

	// Try ~/.volcengine/config.json
	home, err := os.UserHomeDir()
	if err != nil {
		return nil //nolint // best-effort
	}
	cfgPath := filepath.Join(home, ".volcengine", "config.json")
	data, err := os.ReadFile(cfgPath) //nolint:gosec // standard config file path
	if err != nil {
		return nil //nolint // not found — rely on env
	}

	var cfg struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Region          string `json:"region"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if cfg.AccessKeyID != "" {
		_ = os.Setenv("VOLCENGINE_ACCESS_KEY_ID", cfg.AccessKeyID)
		_ = os.Setenv("VOLCENGINE_SECRET_ACCESS_KEY", cfg.SecretAccessKey)
	}
	if cfg.Region != "" {
		_ = os.Setenv("VOLCENGINE_REGION", cfg.Region)
	}
	return nil
}

// ipLess compares two IPv4 address strings numerically.
func ipLess(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := range pa {
		if i >= len(pb) {
			return false
		}
		if pa[i] < pb[i] {
			return true
		}
		if pa[i] > pb[i] {
			return false
		}
	}
	return false
}
