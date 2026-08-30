package cmd

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/pool"
)

const defaultStateDir = "/var/lib/cocoon/net"

var (
	flagPlatform     string
	flagNodeName     string
	flagSubnet       string
	flagPoolSize     int
	flagGateway      string
	flagPrimaryNIC   string
	flagDNS          string
	flagStateDir     string
	flagDryRun       bool
	flagDropInternal bool
	flagDropCIDRs    []string

	// flagManageIPTables inverts node.Config.SkipIPTables; adopt-only, off by default to preserve host rules.
	flagManageIPTables bool
)

func registerCommonFlags(cmd *cobra.Command, defaultPoolSize int) {
	cmd.Flags().StringVar(&flagPlatform, "platform", "", "cloud platform (gke|volcengine); auto-detected from instance metadata if omitted")
	cmd.Flags().StringVar(&flagNodeName, "node-name", "", "virtual node name (required)")
	cmd.Flags().StringVar(&flagSubnet, "subnet", "", "VM subnet CIDR, e.g. 172.20.100.0/24 (required)")
	cmd.Flags().IntVar(&flagPoolSize, "pool-size", defaultPoolSize, "number of IPs in the pool")
	cmd.Flags().StringVar(&flagGateway, "gateway", "", "gateway IP on cni0 (default: first IP in subnet)")
	cmd.Flags().StringVar(&flagPrimaryNIC, "primary-nic", "", "host primary NIC (auto-detect if empty)")
	cmd.Flags().StringVar(&flagDNS, "dns", "8.8.8.8,1.1.1.1", "comma-separated DNS servers for DHCP clients")
	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "state directory")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without making changes")
	cmd.Flags().BoolVar(&flagDropInternal, "drop-internal-access", false, "block VM-to-VM traffic within the cocoon subnet")
	cmd.Flags().StringArrayVar(&flagDropCIDRs, "drop-cidr", nil, "additional external destination CIDR VMs may not reach; repeatable (e.g. --drop-cidr 10.0.0.0/8)")
	_ = cmd.MarkFlagRequired("node-name")
	_ = cmd.MarkFlagRequired("subnet")
}

func loadPoolState(ctx context.Context) (*pool.State, error) {
	state, err := pool.Load(ctx, flagStateDir)
	if err != nil {
		return nil, fmt.Errorf("load pool state: %w (run 'cocoon-net init' first)", err)
	}
	return state, nil
}

func loadPlatformFromState(ctx context.Context) (*pool.State, platform.CloudPlatform, error) {
	state, err := loadPoolState(ctx)
	if err != nil {
		return nil, nil, err
	}
	plat, err := newPlatform(ctx, state.Platform)
	if err != nil {
		return nil, nil, fmt.Errorf("load platform %s: %w", state.Platform, err)
	}
	return state, plat, nil
}

func nodeConfigFromState(state *pool.State, skipIPTables bool) *node.Config {
	return &node.Config{
		Gateway:            state.Gateway,
		SubnetCIDR:         state.Subnet,
		PrimaryNIC:         state.PrimaryNIC,
		SecondaryNICs:      state.SecondaryNICs,
		SkipIPTables:       skipIPTables,
		DropInternalAccess: state.DropInternalAccess,
		DropCIDRs:          state.DropCIDRs,
	}
}

// resolvePlatform fills flagPlatform in place so dry-run output sees the detected name too.
func resolvePlatform(ctx context.Context) error {
	if flagPlatform != "" {
		return nil
	}
	detected, err := detectPlatform(ctx)
	if err != nil {
		return err
	}
	flagPlatform = detected
	return nil
}

// resolveSubnet masks flagSubnet in place so the persisted CIDR matches what the cloud reports back at teardown.
func resolveSubnet() error {
	_, ipNet, err := net.ParseCIDR(flagSubnet)
	if err != nil {
		return fmt.Errorf("parse --subnet %q: %w", flagSubnet, err)
	}
	flagSubnet = ipNet.String()
	return nil
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func parseIPs(strs []string) []net.IP {
	ips := make([]net.IP, 0, len(strs))
	for _, s := range strs {
		if ip := net.ParseIP(s).To4(); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}
