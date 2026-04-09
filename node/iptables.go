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
func setupIPTables(ctx context.Context, subnetCIDR, primaryNIC string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupIPTables")

	// FORWARD rules for each secondary NIC <-> cni0.
	for _, iface := range secondaryNICs {
		if err := iptEnsure(ctx, "FORWARD", "-i", iface, "-o", cni0Bridge, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD %s->cni0: %w", iface, err)
		}
		if err := iptEnsure(ctx, "FORWARD", "-i", cni0Bridge, "-o", iface, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("iptables FORWARD cni0->%s: %w", iface, err)
		}
	}

	// Allow cni0 <-> cni0 (inter-VM on same bridge).
	if err := iptEnsure(ctx, "FORWARD", "-i", cni0Bridge, "-o", cni0Bridge, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables FORWARD cni0<->cni0: %w", err)
	}

	// NAT MASQUERADE for outbound traffic from VM subnet.
	if err := iptNatEnsure(ctx, "POSTROUTING", "-s", subnetCIDR, "!", "-o", cni0Bridge, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("iptables NAT MASQUERADE: %w", err)
	}

	logger.Infof(ctx, "iptables configured for subnet %s", subnetCIDR)
	_ = primaryNIC // reserved for future use
	return nil
}

// iptEnsure adds an iptables rule in the filter table if it does not already exist.
func iptEnsure(ctx context.Context, chain string, args ...string) error {
	checkArgs := append([]string{"-t", "filter", "-C", chain}, args...)
	//nolint:gosec // args come from internal constants
	checkCmd := exec.CommandContext(ctx, "iptables", checkArgs...)
	if checkCmd.Run() == nil {
		return nil // rule already exists
	}
	addArgs := append([]string{"-t", "filter", "-A", chain}, args...)
	//nolint:gosec // args come from internal constants
	addCmd := exec.CommandContext(ctx, "iptables", addArgs...)
	out, err := addCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables -A %s %s: %w: %s", chain, strings.Join(args, " "), err, out)
	}
	return nil
}

// iptNatEnsure adds an iptables rule in the nat table if it does not already exist.
func iptNatEnsure(ctx context.Context, chain string, args ...string) error {
	checkArgs := append([]string{"-t", "nat", "-C", chain}, args...)
	//nolint:gosec // args come from internal constants
	checkCmd := exec.CommandContext(ctx, "iptables", checkArgs...)
	if checkCmd.Run() == nil {
		return nil
	}
	addArgs := append([]string{"-t", "nat", "-A", chain}, args...)
	//nolint:gosec // args come from internal constants
	addCmd := exec.CommandContext(ctx, "iptables", addArgs...)
	out, err := addCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables -t nat -A %s %s: %w: %s", chain, strings.Join(args, " "), err, out)
	}
	return nil
}
