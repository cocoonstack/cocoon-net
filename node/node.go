package node

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/projecteru2/core/log"
)

const (
	maxSecondaryNICs = 7

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
	PrimaryNIC string

	// SkipIPTables omits the iptables FORWARD + NAT MASQUERADE rules.
	SkipIPTables bool
}

// Setup configures host networking components (idempotent):
//  1. cni0 bridge (must exist before sysctl sets per-interface params)
//  2. sysctl (ip_forward, rp_filter)
//  3. iptables FORWARD + NAT
//  4. CNI conflist
//
// Host routes (/32) are NOT added here — they are managed dynamically
// by the DHCP server when leases are granted/released.
func Setup(ctx context.Context, cfg *Config) error {
	logger := log.WithFunc("node.Setup")

	secondaryNICs := detectSecondaryNICs()
	logger.Infof(ctx, "detected secondary NICs: %v", secondaryNICs)

	if err := setupBridge(ctx, cfg.Gateway, cfg.SubnetCIDR); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}

	if err := setupSysctl(ctx, cfg.PrimaryNIC, secondaryNICs); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}

	if cfg.SkipIPTables {
		logger.Info(ctx, "iptables setup skipped (SkipIPTables=true)")
	} else if err := setupIPTables(ctx, cfg.SubnetCIDR, secondaryNICs); err != nil {
		return fmt.Errorf("iptables: %w", err)
	}

	if err := writeCNIConflist(ctx); err != nil {
		return fmt.Errorf("cni conflist: %w", err)
	}

	logger.Info(ctx, "node setup complete")
	return nil
}

// writeCNIConflist writes the dnsmasq-dhcp CNI conflist if content has changed.
func writeCNIConflist(ctx context.Context) error {
	logger := log.WithFunc("node.writeCNIConflist")

	if err := os.MkdirAll(cniConfDir, 0o750); err != nil {
		return fmt.Errorf("create cni conf dir: %w", err)
	}
	confPath := filepath.Join(cniConfDir, cniConfFile)

	if existing, err := os.ReadFile(confPath); err == nil && string(existing) == cniConfContent { //nolint:gosec // known path
		logger.Info(ctx, "CNI conflist unchanged, skipping write")
		return nil
	}

	if err := os.WriteFile(confPath, []byte(cniConfContent), 0o644); err != nil { //nolint:gosec // readable config
		return fmt.Errorf("write cni conflist: %w", err)
	}
	logger.Infof(ctx, "wrote CNI conflist to %s", confPath)
	return nil
}

// detectSecondaryNICs returns the list of secondary NIC names (eth1..ethN) that exist.
func detectSecondaryNICs() []string {
	var nics []string
	for i := 1; i <= maxSecondaryNICs; i++ {
		name := fmt.Sprintf("eth%d", i)
		if _, err := net.InterfaceByName(name); err == nil {
			nics = append(nics, name)
		}
	}
	return nics
}
