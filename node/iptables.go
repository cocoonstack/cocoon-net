package node

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

// setupIPTables installs FORWARD rules between secondary NICs and cni0,
// and a NAT MASQUERADE rule for outbound VM traffic.
func setupIPTables(ctx context.Context, subnetCIDR string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupIPTables")

	for _, iface := range secondaryNICs {
		if err := iptEnsure(ctx, "filter", "FORWARD", "-i", iface, "-o", cni0Bridge, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD %s->cni0: %w", iface, err)
		}
		if err := iptEnsure(ctx, "filter", "FORWARD", "-i", cni0Bridge, "-o", iface, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD cni0->%s: %w", iface, err)
		}
	}

	if err := iptEnsure(ctx, "filter", "FORWARD", "-i", cni0Bridge, "-o", cni0Bridge, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables FORWARD cni0<->cni0: %w", err)
	}

	if err := iptEnsure(ctx, "nat", "POSTROUTING", "-s", subnetCIDR, "!", "-o", cni0Bridge, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("iptables NAT MASQUERADE: %w", err)
	}

	logger.Infof(ctx, "iptables configured for subnet %s", subnetCIDR)
	return nil
}

// iptEnsure adds an iptables rule if it does not already exist.
func iptEnsure(ctx context.Context, table, chain string, args ...string) error {
	checkArgs := append([]string{"-t", table, "-C", chain}, args...)
	//nolint:gosec // args come from internal constants
	checkCmd := exec.CommandContext(ctx, "iptables", checkArgs...)
	if checkCmd.Run() == nil {
		return nil
	}
	addArgs := append([]string{"-t", table, "-A", chain}, args...)
	//nolint:gosec // args come from internal constants
	addCmd := exec.CommandContext(ctx, "iptables", addArgs...)
	out, err := addCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables -t %s -A %s %s: %w: %s", table, chain, strings.Join(args, " "), err, out)
	}
	return nil
}
