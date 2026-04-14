package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/projecteru2/core/log"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-net/dhcp"
	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
	"github.com/cocoonstack/cocoon-net/pool"
)

const (
	defaultLeaseFile = "/var/lib/cocoon/net/leases.json"
	pidFile          = "/run/cocoon-net.pid"
	cni0Bridge       = "cni0"
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a long-lived service: setup node networking and serve DHCP",
		Long: `Daemon mode loads the IP pool from the state file, configures host
networking (sysctl, bridge, iptables), and starts an embedded DHCP server
on cni0. Host routes (/32) are added dynamically when leases are granted
and removed when they expire. This replaces the external dnsmasq dependency.`,
		RunE: runDaemon,
	}
	cmd.Flags().String("state-dir", defaultStateDir, "directory containing pool.json")
	cmd.Flags().String("lease-file", defaultLeaseFile, "path to lease persistence file")
	cmd.Flags().StringSlice("dns", []string{"8.8.8.8", "1.1.1.1"}, "DNS servers for DHCP clients")
	cmd.Flags().Bool("skip-iptables", false, "skip iptables setup (for pre-configured nodes)")
	return cmd
}

func runDaemon(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	logger := log.WithFunc("cmd.daemon")

	if err := acquirePIDFile(); err != nil {
		return err
	}
	defer os.Remove(pidFile) //nolint:errcheck

	stateDir, _ := cmd.Flags().GetString("state-dir")
	leaseFile, _ := cmd.Flags().GetString("lease-file")
	dnsStrs, _ := cmd.Flags().GetStringSlice("dns")
	skipIPTables, _ := cmd.Flags().GetBool("skip-iptables")

	// Load pool state.
	state, err := pool.Load(ctx, stateDir)
	if err != nil {
		return fmt.Errorf("load pool state: %w (run 'cocoon-net init' first)", err)
	}
	logger.Infof(ctx, "pool loaded: %d IPs, subnet %s, gateway %s", len(state.IPs), state.Subnet, state.Gateway)

	// Setup host networking (idempotent).
	primaryNIC := platform.DefaultNIC(state.Platform)
	if setupErr := node.Setup(ctx, &node.Config{
		Gateway:      state.Gateway,
		SubnetCIDR:   state.Subnet,
		PrimaryNIC:   primaryNIC,
		SkipIPTables: skipIPTables,
	}); setupErr != nil {
		return fmt.Errorf("node setup: %w", setupErr)
	}

	// Parse network config.
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
	dnsIPs := parseIPs(dnsStrs)

	// Start DHCP server (blocks until ctx canceled).
	srv := dhcp.New(dhcp.Config{
		Interface:  cni0Bridge,
		Gateway:    gateway,
		SubnetMask: ipNet.Mask,
		DNSServers: dnsIPs,
		LeaseFile:  leaseFile,
	}, poolIPs)

	logger.Info(ctx, "starting DHCP daemon")
	return srv.Run(ctx)
}

// acquirePIDFile writes the current PID to /run/cocoon-net.pid and fails
// if another instance is already running.
func acquirePIDFile() error {
	if err := checkExistingPID(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil { //nolint:gosec
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644) //nolint:gosec
}

func checkExistingPID() error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return nil // no PID file, safe to proceed
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return nil // corrupt PID file, overwrite
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if proc.Signal(syscall.Signal(0)) == nil {
		return fmt.Errorf("another cocoon-net daemon is running (pid %d)", pid)
	}
	return nil // stale PID, process dead
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
