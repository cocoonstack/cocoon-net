package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/projecteru2/core/log"
)

const (
	// BridgeName is the Linux bridge used for VM networking.
	BridgeName = "cni0"

	cniConfDir  = "/etc/cni/net.d"
	cniConfFile = "30-cocoon-dhcp.conflist"
)

// Config holds parameters for node setup.
type Config struct {
	Gateway       string
	SubnetCIDR    string
	PrimaryNIC    string
	SecondaryNICs []string // platform-provided (e.g. Volcengine eth1..eth7)

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

	if len(cfg.SecondaryNICs) > 0 {
		logger.Infof(ctx, "secondary NICs: %v", cfg.SecondaryNICs)
	}

	if err := setupBridge(ctx, cfg.Gateway, cfg.SubnetCIDR); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}

	if err := setupSysctl(ctx, cfg.PrimaryNIC, cfg.SecondaryNICs); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}

	if cfg.SkipIPTables {
		logger.Info(ctx, "iptables setup skipped (SkipIPTables=true)")
	} else if err := setupIPTables(ctx, cfg.SubnetCIDR, cfg.SecondaryNICs); err != nil {
		return fmt.Errorf("iptables: %w", err)
	}

	if err := writeCNIConflist(ctx); err != nil {
		return fmt.Errorf("cni conflist: %w", err)
	}

	logger.Info(ctx, "node setup complete")
	return nil
}

// writeCNIConflist writes the cocoon-dhcp CNI conflist if content has changed.
func writeCNIConflist(ctx context.Context) error {
	logger := log.WithFunc("node.writeCNIConflist")

	content := fmt.Sprintf(`{
  "cniVersion": "1.0.0",
  "name": "cocoon-dhcp",
  "plugins": [
    {
      "type": "bridge",
      "bridge": %q,
      "isGateway": false,
      "ipMasq": false,
      "ipam": {}
    }
  ]
}
`, BridgeName)

	if err := os.MkdirAll(cniConfDir, 0o750); err != nil {
		return fmt.Errorf("create cni conf dir: %w", err)
	}
	confPath := filepath.Join(cniConfDir, cniConfFile)

	if existing, err := os.ReadFile(confPath); err == nil && string(existing) == content { //nolint:gosec // known path
		logger.Info(ctx, "CNI conflist unchanged, skipping write")
		return nil
	}

	if err := os.WriteFile(confPath, []byte(content), 0o644); err != nil { //nolint:gosec // readable config
		return fmt.Errorf("write cni conflist: %w", err)
	}
	logger.Infof(ctx, "wrote CNI conflist to %s", confPath)
	return nil
}
