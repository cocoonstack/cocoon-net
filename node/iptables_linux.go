//go:build linux

package node

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/projecteru2/core/log"
)

// dropRuleComment tags cocoon-net's DROP rules for teardown. Must stay
// quote-safe ([-_+./0-9A-Za-z]) or iptables -S quotes it, breaking removal.
const dropRuleComment = "cocoon-net-drop"

// ClearDropRules removes every FORWARD egress-isolation rule cocoon-net installed.
func ClearDropRules(ctx context.Context) error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("init iptables: %w", err)
	}
	return reconcileDropRules(ctx, ipt, nil)
}

// reconcileDropRules deletes tagged FORWARD drop rules not in want (nil want
// removes all); callers insert desired rules first so the reconcile is gapless.
func reconcileDropRules(ctx context.Context, ipt *iptables.IPTables, want []string) error {
	logger := log.WithFunc("node.reconcileDropRules")

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
		if dst, ok := ruleDest(fields); ok && slices.Contains(want, dst) {
			continue
		}
		if err := ipt.Delete("filter", "FORWARD", fields[2:]...); err != nil {
			failed++
			continue
		}
		removed++
	}

	if removed > 0 {
		logger.Infof(ctx, "removed %d egress drop rule(s)", removed)
	}
	if failed > 0 {
		return fmt.Errorf("delete %d of %d drop rules failed", failed, removed+failed)
	}
	return nil
}

func ruleDest(fields []string) (string, bool) {
	if i := slices.Index(fields, "-d"); i >= 0 && i+1 < len(fields) {
		return fields[i+1], true
	}
	return "", false
}

// setupIPTables installs FORWARD rules between secondary NICs and the bridge, a NAT
// MASQUERADE for outbound VM traffic, and egress DROP rules blocking VM traffic to
// dropCIDRs: L3-routed cross-node/external ranges that need no br_netfilter.
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

	if len(dropTargets) > 0 {
		// Insert at FORWARD's head so DROP precedes the ACCEPT rules; the -i match
		// spares return traffic, VM-to-gateway is INPUT not FORWARD.
		for _, dst := range dropTargets {
			if err := iptInsert(ipt, "filter", "FORWARD", "-i", BridgeName, "-d", dst, "-m", "comment", "--comment", dropRuleComment, "-j", "DROP"); err != nil {
				return fmt.Errorf("iptables FORWARD drop %s: %w", dst, err)
			}
		}
	}

	// Prune rules no longer wanted; desired ones were inserted above, so gapless.
	if err := reconcileDropRules(ctx, ipt, dropTargets); err != nil {
		return fmt.Errorf("reconcile drop rules: %w", err)
	}

	logger.Infof(ctx, "iptables configured for subnet %s, %d egress drop rule(s)", subnetCIDR, len(dropTargets))
	return nil
}

// resolveDropTargets canonicalizes CIDRs to iptables -S form so reconcile can
// match them; IPv6 is rejected because the rules go through the IPv4 binary.
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
