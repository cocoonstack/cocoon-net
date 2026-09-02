package cmd

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/projecteru2/core/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/cocoonstack/cocoon-common/httpx"
	"github.com/cocoonstack/cocoon-net/dhcp"
	"github.com/cocoonstack/cocoon-net/metrics"
	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	defaultLeaseFile     = "/var/lib/cocoon/net/leases.json"
	defaultControlSocket = "/run/cocoon-net/control.sock"
	defaultMetricsAddr   = ":9092"

	metricsShutdownTimeout = 5 * time.Second
)

var (
	fallbackDNSServers = []string{"8.8.8.8", "1.1.1.1"}

	flagLeaseFile     string
	flagControlSocket string
	flagSkipIPTables  bool
	flagMetricsAddr   string
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a long-lived service: setup node networking and serve DHCP",
		Long: `Daemon mode loads the IP pool from the state file, configures host
networking (brings configured secondary NICs up, then applies the bridge,
sysctl, and iptables), and starts an embedded DHCP server on cni0. Host
routes (/32) are added dynamically when leases are granted and removed when
they expire.`,
		RunE: runDaemon,
	}
	cmd.Flags().StringVar(&flagStateDir, "state-dir", defaultStateDir, "directory containing pool.json")
	cmd.Flags().StringVar(&flagLeaseFile, "lease-file", defaultLeaseFile, "path to lease persistence file")
	cmd.Flags().StringVar(&flagControlSocket, "control-socket", cmp.Or(os.Getenv("COCOON_NET_CONTROL_SOCKET"), defaultControlSocket), "root-only Unix socket for local lease lifecycle operations (empty to disable)")
	cmd.Flags().BoolVar(&flagSkipIPTables, "skip-iptables", false, "skip iptables setup (for pre-configured nodes)")
	cmd.Flags().StringVar(&flagMetricsAddr, "metrics-addr", cmp.Or(os.Getenv("COCOON_NET_METRICS_ADDR"), defaultMetricsAddr), "prometheus metrics listen address (empty to disable)")
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

	state.PrimaryNIC = cmp.Or(state.PrimaryNIC, platform.DefaultNIC(state.Platform))
	state.SecondaryNICs = node.PresentLinks(state.SecondaryNICs)
	if setupErr := node.Setup(ctx, nodeConfigFromState(state, flagSkipIPTables)); setupErr != nil {
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
		return errors.New("no valid IPs in pool")
	}
	dnsList := state.DNSServers
	if len(dnsList) == 0 {
		logger.Warnf(ctx, "pool.json has no dnsServers (pre-migration state?); falling back to %v", fallbackDNSServers)
		dnsList = fallbackDNSServers
	}
	dnsIPs := parseIPs(dnsList)

	srv := dhcp.New(dhcp.Config{
		Interface:     node.BridgeName,
		Gateway:       gateway,
		SubnetMask:    ipNet.Mask,
		DNSServers:    dnsIPs,
		LeaseFile:     flagLeaseFile,
		ControlSocket: flagControlSocket,
	}, poolIPs)

	if flagMetricsAddr != "" {
		serveMetrics(ctx, flagMetricsAddr, srv)
	}

	logger.Info(ctx, "starting DHCP daemon")
	return srv.Run(ctx)
}

// serveMetrics never fails the daemon: metrics must not take down live VM networking.
func serveMetrics(ctx context.Context, addr string, srv *dhcp.Server) {
	logger := log.WithFunc("cmd.serveMetrics")

	metrics.Register(prometheus.DefaultRegisterer)
	prometheus.DefaultRegisterer.MustRegister(metrics.NewPoolCollector(func() metrics.PoolState {
		return metrics.PoolState{Available: srv.PoolAvailable(), Active: srv.ActiveLeaseCount()}
	}))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		logger.Infof(ctx, "metrics server listening on %s", addr)
		if err := httpx.Run(ctx, metricsShutdownTimeout, httpx.HTTPServerSpec(httpx.NewServer(addr, mux))); err != nil {
			logger.Error(ctx, err, "metrics server")
		}
	}()
}
