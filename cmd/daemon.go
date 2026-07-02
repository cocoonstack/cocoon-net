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

	"github.com/cocoonstack/cocoon-net/dhcp"
	"github.com/cocoonstack/cocoon-net/metrics"
	"github.com/cocoonstack/cocoon-net/node"
	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	defaultLeaseFile   = "/var/lib/cocoon/net/leases.json"
	defaultMetricsAddr = ":9092"

	metricsReadHeaderTimeout = 5 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
)

var (
	// fallbackDNSServers covers pre-migration state files without DNSServers.
	fallbackDNSServers = []string{"8.8.8.8", "1.1.1.1"}

	flagLeaseFile    string
	flagSkipIPTables bool
	flagMetricsAddr  string
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

	primaryNIC := state.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = platform.DefaultNIC(state.Platform)
	}
	if setupErr := node.Setup(ctx, &node.Config{
		Gateway:            state.Gateway,
		SubnetCIDR:         state.Subnet,
		PrimaryNIC:         primaryNIC,
		SecondaryNICs:      state.SecondaryNICs,
		SkipIPTables:       flagSkipIPTables,
		DropInternalAccess: state.DropInternalAccess,
		DropCIDRs:          state.DropCIDRs,
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
		return errors.New("no valid IPs in pool")
	}
	dnsList := state.DNSServers
	if len(dnsList) == 0 {
		logger.Warnf(ctx, "pool.json has no dnsServers (pre-migration state?); falling back to %v", fallbackDNSServers)
		dnsList = fallbackDNSServers
	}
	dnsIPs := parseIPs(dnsList)

	srv := dhcp.New(dhcp.Config{
		Interface:  node.BridgeName,
		Gateway:    gateway,
		SubnetMask: ipNet.Mask,
		DNSServers: dnsIPs,
		LeaseFile:  flagLeaseFile,
	}, poolIPs)

	if flagMetricsAddr != "" {
		serveMetrics(ctx, flagMetricsAddr, srv)
	}

	logger.Info(ctx, "starting DHCP daemon")
	return srv.Run(ctx)
}

// serveMetrics failures are logged, never fatal — metrics must not take down
// live VM networking.
func serveMetrics(ctx context.Context, addr string, srv *dhcp.Server) {
	logger := log.WithFunc("cmd.serveMetrics")

	metrics.Register(prometheus.DefaultRegisterer)
	prometheus.DefaultRegisterer.MustRegister(metrics.NewPoolCollector(func() metrics.PoolState {
		return metrics.PoolState{Available: srv.PoolAvailable(), Active: srv.ActiveLeaseCount()}
	}))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: metricsReadHeaderTimeout}

	go func() {
		<-ctx.Done()
		// New ctx: the parent is already canceled, but shutdown must still drain.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		logger.Infof(ctx, "metrics server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, err, "metrics server")
		}
	}()
}
