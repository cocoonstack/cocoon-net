package node

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/projecteru2/core/log"
)

const (
	cniConfDir     = "/etc/cni/net.d"
	cniConfFile    = "30-dnsmasq-dhcp.conflist"
	cniConfContent = `{
  "cniVersion": "1.0.0",
  "name": "dnsmasq-dhcp",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": false,
      "ipMasq": false,
      "ipam": {}
    }
  ]
}
`
)

// Config holds parameters for node setup.
type Config struct {
	Gateway    string
	SubnetCIDR string
	IPs        []string
	DNSServers []string
	PrimaryNIC string
}

// Setup configures all node networking components in order:
//  1. sysctl
//  2. cni0 bridge
//  3. host routes
//  4. iptables
//  5. dnsmasq
//  6. CNI conflist
func Setup(ctx context.Context, cfg *Config) error {
	logger := log.WithFunc("node.Setup")

	// Detect secondary NICs (eth1..eth7 that are present).
	secondaryNICs := detectSecondaryNICs()
	logger.Infof(ctx, "detected secondary NICs: %v", secondaryNICs)

	// 1. sysctl.
	if err := setupSysctl(ctx, cfg.PrimaryNIC, secondaryNICs); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}

	// 2. cni0 bridge.
	if err := setupBridge(ctx, cfg.Gateway, cfg.SubnetCIDR); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}

	// 3. Host routes.
	if err := setupRoutes(ctx, cfg.IPs); err != nil {
		return fmt.Errorf("routes: %w", err)
	}

	// 4. iptables.
	if err := setupIPTables(ctx, cfg.SubnetCIDR, cfg.PrimaryNIC, secondaryNICs); err != nil {
		return fmt.Errorf("iptables: %w", err)
	}

	// 5. dnsmasq.
	if err := setupDNSMasq(ctx, cfg.Gateway, cfg.SubnetCIDR, cfg.IPs, cfg.DNSServers, cfg.PrimaryNIC); err != nil {
		return fmt.Errorf("dnsmasq: %w", err)
	}

	// 6. CNI conflist.
	if err := writeCNIConflist(ctx); err != nil {
		return fmt.Errorf("cni conflist: %w", err)
	}

	logger.Info(ctx, "node setup complete")
	return nil
}

// writeCNIConflist writes the dnsmasq-dhcp CNI conflist.
func writeCNIConflist(ctx context.Context) error {
	logger := log.WithFunc("node.writeCNIConflist")

	if err := os.MkdirAll(cniConfDir, 0o750); err != nil {
		return fmt.Errorf("create cni conf dir: %w", err)
	}
	confPath := filepath.Join(cniConfDir, cniConfFile)
	if err := os.WriteFile(confPath, []byte(cniConfContent), 0o644); err != nil { //nolint:gosec // readable config
		return fmt.Errorf("write cni conflist: %w", err)
	}
	logger.Infof(ctx, "wrote CNI conflist to %s", confPath)
	return nil
}

// detectSecondaryNICs returns the list of secondary NIC names (eth1..eth7) that exist.
func detectSecondaryNICs() []string {
	var nics []string
	for i := 1; i <= 7; i++ {
		name := fmt.Sprintf("eth%d", i)
		out, err := exec.Command("ip", "link", "show", name).CombinedOutput() //nolint:gosec // ip args from trusted NIC name
		if err == nil && strings.Contains(string(out), name) {
			nics = append(nics, name)
		}
	}
	return nics
}
