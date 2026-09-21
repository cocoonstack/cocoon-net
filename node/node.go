// Package node configures node-local networking: secondary NICs, bridge, sysctls, iptables, and the CNI conflist.
package node

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/metrics"
)

const (
	// BridgeName is the Linux bridge used for VM networking.
	BridgeName = "cni0"

	cniConfDir  = "/etc/cni/net.d"
	cniConfFile = "30-cocoon-dhcp.conflist"

	filePerm = 0o644
	dirPerm  = 0o750
)

// Config holds parameters for node setup.
type Config struct {
	Gateway       string
	SubnetCIDR    string
	PrimaryNIC    string
	SecondaryNICs []string // platform-provided (e.g. Volcengine eth1..eth7)

	// SkipIPTables omits the iptables FORWARD + NAT MASQUERADE rules.
	SkipIPTables bool

	// DropInternalAccess adds a FORWARD DROP for SubnetCIDR (cross-node; same-node isolation is L2 portIsolation).
	DropInternalAccess bool

	// DropCIDRs adds FORWARD DROPs for extra destination CIDRs; ignored with SkipIPTables like DropInternalAccess.
	DropCIDRs []string
}

// Setup configures host networking idempotently; the bridge must exist before sysctl touches it.
func Setup(ctx context.Context, cfg *Config) error {
	logger := log.WithFunc("node.Setup")

	nics, err := usableSecondaryNICs(ctx, cfg.SecondaryNICs, PresentLinks(cfg.SecondaryNICs))
	if err != nil {
		return err
	}
	if len(nics) > 0 {
		logger.Infof(ctx, "secondary NICs: %v", nics)
	}
	if err := setupSecondaryNICs(nics); err != nil {
		return fmt.Errorf("secondary NICs: %w", err)
	}

	if err := setupBridge(ctx, cfg.Gateway, cfg.SubnetCIDR); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}

	if err := setupSysctl(ctx, cfg.PrimaryNIC, nics); err != nil {
		return fmt.Errorf("sysctl: %w", err)
	}

	if cfg.SkipIPTables {
		logger.Info(ctx, "iptables setup skipped (SkipIPTables=true)")
	} else if err := setupIPTables(ctx, cfg.SubnetCIDR, nics, cfg.DropInternalAccess, cfg.DropCIDRs); err != nil {
		return fmt.Errorf("iptables: %w", err)
	}

	if err := writeCNIConflist(ctx); err != nil {
		return fmt.Errorf("cni conflist: %w", err)
	}

	logger.Info(ctx, "node setup complete")
	return nil
}

func writeCNIConflist(ctx context.Context) error {
	logger := log.WithFunc("node.writeCNIConflist")

	conflist := map[string]any{
		"cniVersion": "1.0.0",
		"name":       "cocoon-dhcp",
		"plugins": []map[string]any{
			{
				"type":          "bridge",
				"bridge":        BridgeName,
				"isGateway":     false,
				"ipMasq":        false,
				"portIsolation": true, // block same-node VM-to-VM at L2 (BR_ISOLATED per veth)
				"macspoofchk":   true, // pin veth source MAC (anti-spoof)
				"ipam":          map[string]any{},
			},
		},
	}
	encoded, err := json.MarshalIndent(conflist, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cni conflist: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := os.MkdirAll(cniConfDir, dirPerm); err != nil {
		return fmt.Errorf("create cni conf dir: %w", err)
	}
	confPath := filepath.Join(cniConfDir, cniConfFile)

	if existing, err := os.ReadFile(confPath); err == nil && string(existing) == string(encoded) { //nolint:gosec // known path
		logger.Info(ctx, "CNI conflist unchanged, skipping write")
		return nil
	}

	if err := os.WriteFile(confPath, encoded, filePerm); err != nil {
		return fmt.Errorf("write cni conflist: %w", err)
	}
	logger.Infof(ctx, "wrote CNI conflist to %s", confPath)
	return nil
}

func usableSecondaryNICs(ctx context.Context, expected, present []string) ([]string, error) {
	logger := log.WithFunc("node.usableSecondaryNICs")
	metrics.SecondaryNICs.WithLabelValues("expected").Set(float64(len(expected)))
	metrics.SecondaryNICs.WithLabelValues("present").Set(float64(len(present)))
	if len(expected) > 0 && len(present) == 0 {
		return nil, fmt.Errorf("none of the secondary NICs %v is present", expected)
	}
	for _, nic := range expected {
		if !slices.Contains(present, nic) {
			logger.Warnf(ctx, "secondary NIC %s is missing; the pool IPs behind it stay unreachable until it is re-attached", nic)
		}
	}
	return present, nil
}
