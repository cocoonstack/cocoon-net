//go:build linux

package node

import (
	"context"
	"fmt"

	"github.com/coreos/go-iptables/iptables"
	"github.com/projecteru2/core/log"
)

// setupIPTables installs FORWARD rules between secondary NICs and the bridge,
// and a NAT MASQUERADE rule for outbound VM traffic.
func setupIPTables(ctx context.Context, subnetCIDR string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupIPTables")

	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("init iptables: %w", err)
	}

	for _, iface := range secondaryNICs {
		if err := iptEnsure(ipt, "filter", "FORWARD", "-i", iface, "-o", BridgeName, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD %s->%s: %w", iface, BridgeName, err)
		}
		if err := iptEnsure(ipt, "filter", "FORWARD", "-i", BridgeName, "-o", iface, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD %s->%s: %w", BridgeName, iface, err)
		}
	}

	if err := iptEnsure(ipt, "filter", "FORWARD", "-i", BridgeName, "-o", BridgeName, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables FORWARD %s<->%s: %w", BridgeName, BridgeName, err)
	}

	if err := iptEnsure(ipt, "nat", "POSTROUTING", "-s", subnetCIDR, "!", "-o", BridgeName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("iptables NAT MASQUERADE: %w", err)
	}

	logger.Infof(ctx, "iptables configured for subnet %s", subnetCIDR)
	return nil
}

// iptEnsure adds an iptables rule if it does not already exist.
func iptEnsure(ipt *iptables.IPTables, table, chain string, args ...string) error {
	exists, err := ipt.Exists(table, chain, args...)
	if err != nil {
		return fmt.Errorf("check rule: %w", err)
	}
	if exists {
		return nil
	}
	return ipt.Append(table, chain, args...)
}
