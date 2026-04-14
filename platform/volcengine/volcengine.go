package volcengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

var (
	_ platform.CloudPlatform = (*Volcengine)(nil)

	envOnce sync.Once
	envErr  error
)

const (
	metadataBase = "http://100.96.0.96/latest/meta-data"
	defaultNIC   = "eth0"
	enisPerNode  = 7
	ipsPerENI    = 20

	eniTypePrimary = "primary"

	metadataTimeout        = 2 * time.Second
	createPropagationDelay = 2 * time.Second
	attachPropagationDelay = 4 * time.Second
)

// Volcengine implements CloudPlatform for Volcengine.
type Volcengine struct{}

// Name returns the platform identifier.
func (v *Volcengine) Name() string { return platform.PlatformVolcengine }

// ProvisionNetwork provisions Volcengine networking resources.
func (v *Volcengine) ProvisionNetwork(ctx context.Context, cfg *platform.Config) (*platform.NetworkResult, error) {
	logger := log.WithFunc("volcengine.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = defaultNIC
	}

	if err := setupEnv(); err != nil {
		return nil, fmt.Errorf("setup volcengine credentials: %w", err)
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
			logger.Warnf(ctx, "assign secondary IPs to %s: %v", eniID, assignErr)
			continue
		}
		allIPs = append(allIPs, ips...)
	}
	if len(allIPs) == 0 {
		return nil, fmt.Errorf("no secondary IPs assigned across %d ENIs", len(eniIDs))
	}
	logger.Infof(ctx, "assigned %d secondary IPs", len(allIPs))

	// Bring up secondary interfaces.
	for i := 1; i <= enisPerNode; i++ {
		iface := fmt.Sprintf("eth%d", i)
		//nolint:gosec // iface from trusted integer
		cmd := exec.CommandContext(ctx, "ip", "link", "set", iface, "up")
		out, linkErr := cmd.CombinedOutput()
		if linkErr != nil {
			logger.Warnf(ctx, "bring up %s: %v: %s", iface, linkErr, out)
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

	var secondaryNICs []string
	for i := 1; i <= enisPerNode; i++ {
		secondaryNICs = append(secondaryNICs, fmt.Sprintf("eth%d", i))
	}

	return &platform.NetworkResult{
		Platform:      v.Name(),
		SubnetCIDR:    cfg.SubnetCIDR,
		Gateway:       gateway,
		PrimaryNIC:    primaryNIC,
		SecondaryNICs: secondaryNICs,
		IPs:           allIPs,
	}, nil
}

// Status returns the current ENI and IP status.
func (v *Volcengine) Status(ctx context.Context) (*platform.PoolStatus, error) {
	logger := log.WithFunc("volcengine.Status")

	if err := setupEnv(); err != nil {
		return nil, fmt.Errorf("setup credentials: %w", err)
	}

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

// Teardown detaches and deletes all secondary ENIs for this instance.
func (v *Volcengine) Teardown(ctx context.Context) error {
	logger := log.WithFunc("volcengine.Teardown")

	if err := setupEnv(); err != nil {
		return fmt.Errorf("setup credentials: %w", err)
	}

	instanceID, err := fetchMeta(ctx, "/instance-id")
	if err != nil {
		return fmt.Errorf("get instance id: %w", err)
	}

	enis, err := listENIs(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("list ENIs: %w", err)
	}

	for _, eni := range enis {
		if eni.Type == eniTypePrimary {
			continue
		}

		_, detachErr := veRun(ctx, "vpc", "DetachNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
			"--InstanceId", instanceID,
		)
		if detachErr != nil {
			logger.Warnf(ctx, "detach ENI %s: %v (skipping delete)", eni.NetworkInterfaceID, detachErr)
			continue
		}

		// Wait for detach to propagate before deleting.
		time.Sleep(attachPropagationDelay)

		_, delErr := veRun(ctx, "vpc", "DeleteNetworkInterface",
			"--NetworkInterfaceId", eni.NetworkInterfaceID,
		)
		if delErr != nil {
			logger.Warnf(ctx, "delete ENI %s: %v", eni.NetworkInterfaceID, delErr)
		} else {
			logger.Infof(ctx, "deleted ENI %s", eni.NetworkInterfaceID)
		}
	}
	return nil
}

// Detect probes the Volcengine metadata endpoint.
func Detect(ctx context.Context) bool {
	client := &http.Client{Timeout: metadataTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataBase+"/instance-id", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// --- Volcengine API helpers ---

type networkInterface struct {
	NetworkInterfaceID string `json:"NetworkInterfaceId"`
	Type               string `json:"Type"`
	PrivateIPSets      struct {
		PrivateIPSet []struct {
			Primary          bool   `json:"Primary"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
		} `json:"PrivateIpSet"`
	} `json:"PrivateIpSets"`
}

// fetchMeta fetches a value from the Volcengine instance metadata service.
func fetchMeta(ctx context.Context, path string) (string, error) {
	client := &http.Client{Timeout: metadataTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataBase+path, nil)
	if err != nil {
		return "", fmt.Errorf("create request for %s: %w", path, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	val := strings.TrimSpace(string(b))
	if val == "" {
		return "", fmt.Errorf("%s returned empty", path)
	}
	return val, nil
}

func veRun(ctx context.Context, args ...string) ([]byte, error) {
	//nolint:gosec // args from internal constants and metadata
	cmd := exec.CommandContext(ctx, "ve", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ve %s: %w: %s", strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}

func getSecurityGroupID(ctx context.Context, vpcID string) (string, error) {
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

func ensureSubnet(ctx context.Context, vpcID, cidr, name string) (string, error) {
	out, err := veRun(ctx, "vpc", "DescribeSubnets",
		"--VpcId", vpcID, "--PageSize", "100",
	)
	if err != nil {
		return "", fmt.Errorf("describe subnets: %w", err)
	}
	var listResp struct {
		Result struct {
			Subnets []struct {
				SubnetID  string `json:"SubnetId"`
				CIDRBlock string `json:"CidrBlock"`
			} `json:"Subnets"`
		} `json:"Result"`
	}
	if unmarshalErr := json.Unmarshal(out, &listResp); unmarshalErr != nil {
		return "", fmt.Errorf("parse describe subnets: %w", unmarshalErr)
	}
	for _, s := range listResp.Result.Subnets {
		if s.CIDRBlock == cidr {
			return s.SubnetID, nil
		}
	}

	zone, err := fetchMeta(ctx, "/placement/availability-zone")
	if err != nil {
		return "", fmt.Errorf("fetch zone: %w", err)
	}

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

func createAndAttachENIs(ctx context.Context, subnetID, sgID, instanceID, prefix string, count int) ([]string, error) {
	logger := log.WithFunc("volcengine.createAndAttachENIs")
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

		time.Sleep(createPropagationDelay)

		_, attachErr := veRun(ctx, "vpc", "AttachNetworkInterface",
			"--NetworkInterfaceId", eniID,
			"--InstanceId", instanceID,
		)
		if attachErr != nil {
			logger.Warnf(ctx, "attach ENI %s: %v", eniID, attachErr)
		}

		time.Sleep(attachPropagationDelay)

		eniIDs = append(eniIDs, eniID)
		logger.Infof(ctx, "created and attached ENI %s (%d/%d)", eniID, i, count)
	}
	return eniIDs, nil
}

func assignSecondaryIPs(ctx context.Context, eniID string, count int) ([]string, error) {
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

func listENIs(ctx context.Context, instanceID string) ([]networkInterface, error) {
	out, err := veRun(ctx, "vpc", "DescribeNetworkInterfaces",
		"--InstanceId", instanceID,
		"--PageSize", "100",
	)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			NetworkInterfaceSets []networkInterface `json:"NetworkInterfaceSets"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse ENI list: %w", err)
	}
	return resp.Result.NetworkInterfaceSets, nil
}

// setupEnv reads Volcengine credentials from config file or env vars.
// It is safe to call multiple times; the actual work runs at most once.
func setupEnv() error {
	envOnce.Do(func() {
		envErr = loadEnv()
	})
	return envErr
}

func loadEnv() error {
	if os.Getenv("VOLCENGINE_ACCESS_KEY_ID") != "" {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil //nolint:nilerr // no home dir means no config file to read
	}
	cfgPath := filepath.Join(home, ".volcengine", "config.json")
	data, err := os.ReadFile(cfgPath) //nolint:gosec // standard config file path
	if err != nil {
		return nil //nolint:nilerr // missing config file is not an error; ve CLI will use its own defaults
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
