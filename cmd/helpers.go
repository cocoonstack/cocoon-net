package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

const defaultStateDir = "/var/lib/cocoon/net"

var (
	flagPlatform   string
	flagNodeName   string
	flagSubnet     string
	flagPoolSize   int
	flagGateway    string
	flagPrimaryNIC string
	flagDNS        string
	flagStateDir   string
	flagDryRun     bool
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

// splitTrim splits s by sep and trims whitespace from each element.
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
