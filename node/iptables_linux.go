//go:build linux

package node

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/projecteru2/core/log"
)

// dropRuleComment tags cocoon-net's DROP rules for teardown. Must stay
// quote-safe ([-_+./0-9A-Za-z]) or iptables -S quotes it, breaking removal.
const dropRuleComment = "cocoon-net-drop"

// ClearDropRules removes the FORWARD egress-isolation rules cocoon-net installed.
func ClearDropRules(ctx context.Context) error {
	logger := log.WithFunc("node.ClearDropRules")

	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("init iptables: %w", err)
	}
	rules, err := ipt.List("filter", "FORWARD")
	if err != nil {
		return fmt.Errorf("list FORWARD: %w", err)
	}

	removed, failed := 0, 0
	for _, rule := range rules {
		if !strings.Contains(rule, dropRuleComment) {
			continue
		}
		// List emits "-A FORWARD <spec>"; Delete wants only <spec>.
		fields := strings.Fields(rule)
		if len(fields) < 3 {
			continue
		}
		if err := ipt.Delete("filter", "FORWARD", fields[2:]...); err != nil {
			failed++
			continue
		}
		removed++
	}

	logger.Infof(ctx, "cleared %d egress drop rule(s)", removed)
	if failed > 0 {
		return fmt.Errorf("delete %d of %d drop rules failed", failed, removed+failed)
	}
	return nil
}

// setupIPTables installs the FORWARD rules between secondary NICs and the
// bridge, a NAT MASQUERADE rule for outbound VM traffic, and egress DROP rules
// isolating VMs from their own subnet (dropInternal) and from dropCIDRs.
func setupIPTables(ctx context.Context, subnetCIDR string, secondaryNICs []string, dropInternal bool, dropCIDRs []string) error {
	logger := log.WithFunc("node.setupIPTables")

	// Resolve and validate the drop targets before installing any rule, so a
	// bad CIDR fails without leaving the chain half-configured.
	dropTargets, err := resolveDropTargets(subnetCIDR, dropInternal, dropCIDRs)
	if err != nil {
		return err
	}

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

	if len(dropTargets) == 0 {
		logger.Infof(ctx, "iptables configured for subnet %s", subnetCIDR)
		return nil
	}

	// Same-node VMs share cni0 and are switched at L2, which bypasses iptables
	// unless bridge-nf-call-iptables is on. Without it the DROP rules below
	// would silently not apply to VM-to-VM, so enable it (fail closed) first.
	if err := ensureBridgeNFCall(ctx); err != nil {
		return fmt.Errorf("enable bridge netfilter: %w", err)
	}

	// Insert at the head of FORWARD so DROP wins over the ACCEPT rules above.
	// The -i BridgeName match leaves return traffic (no -i cni0) alone; VM
	// traffic to the gateway is host-bound via INPUT, not FORWARD, so unaffected.
	for _, dst := range dropTargets {
		if err := iptInsert(ipt, "filter", "FORWARD", "-i", BridgeName, "-d", dst, "-m", "comment", "--comment", dropRuleComment, "-j", "DROP"); err != nil {
			return fmt.Errorf("iptables FORWARD drop %s: %w", dst, err)
		}
	}

	logger.Infof(ctx, "iptables configured for subnet %s, %d egress drop rule(s)", subnetCIDR, len(dropTargets))
	return nil
}

// resolveDropTargets resolves the CIDRs VM egress is blocked from reaching: the
// subnet itself when dropInternal is set (VM-to-VM isolation, reusing the range
// cocoon already knows), plus operator-supplied dropCIDRs. CIDRs are
// canonicalized so iptInsert's existence check dedups; IPv6 is rejected because
// the rules go through the IPv4 iptables binary.
func resolveDropTargets(subnetCIDR string, dropInternal bool, dropCIDRs []string) ([]string, error) {
	var raw []string
	if dropInternal {
		raw = append(raw, subnetCIDR)
	}
	raw = append(raw, dropCIDRs...)

	out := make([]string, 0, len(raw))
	for _, cidr := range raw {
		ip, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid drop CIDR %q: %w", cidr, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("drop CIDR %q must be IPv4", cidr)
		}
		out = append(out, ipNet.String())
	}
	return out, nil
}

// ensureBridgeNFCall loads br_netfilter and turns on bridge-nf-call-iptables so
// same-bridge (same-node VM-to-VM) frames traverse iptables. It verifies the
// toggle stuck, failing closed rather than leaving DROP rules the kernel would
// quietly skip for L2-switched traffic.
func ensureBridgeNFCall(ctx context.Context) error {
	logger := log.WithFunc("node.ensureBridgeNFCall")

	// modprobe is the authoritative loader for a kernel module and its deps,
	// with no stdlib equivalent; it is a no-op when br_netfilter is loaded.
	logger.Debug(ctx, "running modprobe br_netfilter (external binary)")
	if out, err := exec.CommandContext(ctx, "modprobe", "br_netfilter").CombinedOutput(); err != nil {
		return fmt.Errorf("modprobe br_netfilter (%s): %w", strings.TrimSpace(string(out)), err)
	}

	const key = "net.bridge.bridge-nf-call-iptables"
	if err := writeSysctl(key, "1"); err != nil {
		return fmt.Errorf("write sysctl %s=1: %w", key, err)
	}
	got, err := readSysctl(key)
	if err != nil {
		return fmt.Errorf("read sysctl %s: %w", key, err)
	}
	if got != "1" {
		return fmt.Errorf("sysctl %s is %q after write, want 1", key, got)
	}

	logger.Info(ctx, "br_netfilter loaded, bridge-nf-call-iptables=1")
	return nil
}

// readSysctl reads a sysctl value via /proc/sys, trimming surrounding whitespace.
func readSysctl(key string) (string, error) {
	b, err := os.ReadFile(sysctlPath(key)) //nolint:gosec // sysctl read of a known proc path
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// iptEnsure appends an iptables rule if it does not already exist.
func iptEnsure(ipt *iptables.IPTables, table, chain string, args ...string) error {
	exists, err := ipt.Exists(table, chain, args...)
	if err != nil {
		return fmt.Errorf("check rule: %w", err)
	}
	if exists {
		return nil
	}
	if err := ipt.Append(table, chain, args...); err != nil {
		return fmt.Errorf("append rule: %w", err)
	}
	return nil
}

// iptInsert inserts an iptables rule at the head of the chain if it does not
// already exist, so the rule takes precedence over appended rules.
func iptInsert(ipt *iptables.IPTables, table, chain string, args ...string) error {
	exists, err := ipt.Exists(table, chain, args...)
	if err != nil {
		return fmt.Errorf("check rule: %w", err)
	}
	if exists {
		return nil
	}
	if err := ipt.Insert(table, chain, 1, args...); err != nil {
		return fmt.Errorf("insert rule: %w", err)
	}
	return nil
}
