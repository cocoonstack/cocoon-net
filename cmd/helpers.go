package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/pool"
)

const defaultStateDir = "/var/lib/cocoon/net"

var (
	// platform / node / subnet
	flagPlatform string
	flagNodeName string
	flagSubnet   string

	// pool
	flagPoolSize int

	// nic
	flagGateway    string
	flagPrimaryNIC string

	// dns
	flagDNS string

	// state
	flagStateDir string

	// debug / misc
	flagDryRun bool

	// flagManageIPTables is the inverse of node.Config.SkipIPTables exposed only
	// on the adopt subcommand: by default adopt preserves the host's existing
	// firewall rules, and the operator must opt in with --manage-iptables to
	// have cocoon-net rewrite them.
	flagManageIPTables bool
)

// registerCommonFlags binds the flags shared by init and adopt subcommands.
func registerCommonFlags(cmd *cobra.Command, defaultPoolSize int) {
	cmd.Flags().StringVar(&flagPlatform, "platform", "", "cloud platform (gke|volcengine)")
	_ = cmd.MarkFlagRequired("platform")
	cmd.Flags().StringVar(&flagNodeName, "node-name", "", "virtual node name (required)")
	cmd.Flags().StringVar(&flagSubnet, "subnet", "", "VM subnet CIDR, e.g. 172.20.100.0/24 (required)")
	cmd.Flags().IntVar(&flagPoolSize, "pool-size", defaultPoolSize, "number of IPs in the pool")
	cmd.Flags().StringVar(&flagGateway, "gateway", "", "gateway IP on cni0 (default: first IP in subnet)")
	cmd.Flags().StringVar(&flagPrimaryNIC, "primary-nic", "", "host primary NIC (auto-detect if empty)")
	cmd.Flags().StringVar(&flagDNS, "dns", "8.8.8.8,1.1.1.1", "comma-separated DNS servers for DHCP clients")
	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "state directory")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without making changes")
	_ = cmd.MarkFlagRequired("node-name")
	_ = cmd.MarkFlagRequired("subnet")
}

// loadPoolState loads the persisted pool.json from flagStateDir and wraps
// the not-found error with a hint to run init/adopt first.
func loadPoolState(ctx context.Context) (*pool.State, error) {
	state, err := pool.Load(ctx, flagStateDir)
	if err != nil {
		return nil, fmt.Errorf("load pool state: %w (run 'cocoon-net init' first)", err)
	}
	return state, nil
}

// splitTrim splits s by sep and trims whitespace from each element.
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// parseIPs converts a string slice to IPv4 addresses, skipping invalid entries.
func parseIPs(strs []string) []net.IP {
	ips := make([]net.IP, 0, len(strs))
	for _, s := range strs {
		if ip := net.ParseIP(s).To4(); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}
