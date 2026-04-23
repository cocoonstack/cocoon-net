package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/dhcp"
	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
)

const defaultLeaseFile = "/var/lib/cocoon/net/leases.json"

var (
	flagLeaseFile    string
	flagDNSSlice     []string
	flagSkipIPTables bool
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a long-lived service: setup node networking and serve DHCP",
		Long: `Daemon mode loads the IP pool from the state file, configures host
networking (sysctl, bridge, iptables), and starts an embedded DHCP server
on cni0. Host routes (/32) are added dynamically when leases are granted
and removed when they expire.`,
		RunE: runDaemon,
	}
	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "directory containing pool.json")
	cmd.Flags().StringVar(&flagLeaseFile, "lease-file", defaultLeaseFile, "path to lease persistence file")
	cmd.Flags().StringSliceVar(&flagDNSSlice, "dns", []string{"8.8.8.8", "1.1.1.1"}, "DNS servers for DHCP clients")
	cmd.Flags().BoolVar(&flagSkipIPTables, "skip-iptables", false, "skip iptables setup (for pre-configured nodes)")
	return cmd
}

func runDaemon(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.runDaemon")

	if err := acquirePIDFile(); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(pidFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warnf(ctx, "remove pid file %s: %v", pidFile, err)
		}
	}()

	state, err := loadPoolState(ctx)
	if err != nil {
		return err
	}
	logger.Infof(ctx, "pool loaded: %d IPs, subnet %s, gateway %s", len(state.IPs), state.Subnet, state.Gateway)

	// Setup host networking (idempotent).
	primaryNIC := state.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = platform.DefaultNIC(state.Platform)
	}
	if setupErr := node.Setup(ctx, &node.Config{
		Gateway:       state.Gateway,
		SubnetCIDR:    state.Subnet,
		PrimaryNIC:    primaryNIC,
		SecondaryNICs: state.SecondaryNICs,
		SkipIPTables:  flagSkipIPTables,
	}); setupErr != nil {
		return fmt.Errorf("node setup: %w", setupErr)
	}

	gateway := net.ParseIP(state.Gateway).To4()
	if gateway == nil {
		return fmt.Errorf("invalid gateway: %s", state.Gateway)
	}

	_, ipNet, err := net.ParseCIDR(state.Subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}

	poolIPs := parseIPs(state.IPs)
	if len(poolIPs) == 0 {
		return fmt.Errorf("no valid IPs in pool")
	}
	dnsIPs := parseIPs(flagDNSSlice)

	// Start DHCP server (blocks until ctx canceled).
	srv := dhcp.New(dhcp.Config{
		Interface:  node.BridgeName,
		Gateway:    gateway,
		SubnetMask: ipNet.Mask,
		DNSServers: dnsIPs,
		LeaseFile:  flagLeaseFile,
	}, poolIPs)

	logger.Info(ctx, "starting DHCP daemon")
	return srv.Run(ctx)
}
