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

	// SkipIPTables omits the iptables FORWARD + NAT MASQUERADE rules. Set this
	// when adopting an existing manually-provisioned node whose firewall rules
	// were tuned by hand and where cocoon-net's MASQUERADE would change the
	// outbound source IP visible to peers (e.g. GKE alias-IP routing already
	// makes per-VM source IPs reachable via the host NIC, and a blanket
	// MASQUERADE rewrite would mask them as the host's own IP). Other steps
	// (sysctl, bridge, routes, dnsmasq, CNI conflist) still run.
	SkipIPTables bool
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

	secondaryNICs := detectSecondaryNICs()
	logger.Infof(ctx, "detected secondary NICs: %v", secondaryNICs)

	if err := setupSysctl(ctx, cfg.PrimaryNIC, secondaryNICs); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}

	if err := setupBridge(ctx, cfg.Gateway, cfg.SubnetCIDR); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}

	if err := setupRoutes(ctx, cfg.IPs); err != nil {
		return fmt.Errorf("routes: %w", err)
	}

	if cfg.SkipIPTables {
		logger.Info(ctx, "iptables setup skipped (SkipIPTables=true)")
	} else if err := setupIPTables(ctx, cfg.SubnetCIDR, secondaryNICs); err != nil {
		return fmt.Errorf("iptables: %w", err)
	}

	if err := setupDNSMasq(ctx, cfg); err != nil {
		return fmt.Errorf("dnsmasq: %w", err)
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

// detectSecondaryNICs returns the list of secondary NIC names (eth1..eth7) that exist.
func detectSecondaryNICs() []string {
	var nics []string
	for i := 1; i <= 7; i++ {
		name := fmt.Sprintf("eth%d", i)
		if _, err := net.InterfaceByName(name); err == nil {
			nics = append(nics, name)
		}
	}
	return nics
}
