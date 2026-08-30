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

// dropRuleComment must stay quote-safe or iptables -S quotes it and teardown cannot match the rule.
const dropRuleComment = "cocoon-net-drop"

// ClearDropRules removes every FORWARD egress-isolation rule cocoon-net installed.
func ClearDropRules(ctx context.Context) error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("init iptables: %w", err)
	}
	return reconcileDropRules(ctx, ipt, nil)
}

// reconcileDropRules deletes tagged FORWARD drop rules not in want; nil want removes them all.
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

// setupIPTables installs the NIC<->bridge FORWARD accepts, the NAT MASQUERADE, and the egress DROPs for dropCIDRs.
func setupIPTables(ctx context.Context, subnetCIDR string, secondaryNICs []string, dropInternal bool, dropCIDRs []string) error {
	logger := log.WithFunc("node.setupIPTables")

	// validate every drop target first so a bad CIDR cannot leave the chain half-configured
	dropTargets, err := resolveDropTargets(subnetCIDR, dropInternal, dropCIDRs)
	if err != nil {
		return err
	}

	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("init iptables: %w", err)
	}

	for _, iface := range secondaryNICs {
		if err := ensureIPTRule(ipt, "filter", "FORWARD", false, "-i", iface, "-o", BridgeName, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("add FORWARD %s->%s: %w", iface, BridgeName, err)
		}
		if err := ensureIPTRule(ipt, "filter", "FORWARD", false, "-i", BridgeName, "-o", iface, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("add FORWARD %s->%s: %w", BridgeName, iface, err)
		}
	}

	if err := ensureIPTRule(ipt, "filter", "FORWARD", false, "-i", BridgeName, "-o", BridgeName, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("add FORWARD %s<->%s: %w", BridgeName, BridgeName, err)
	}

	if err := ensureIPTRule(ipt, "nat", "POSTROUTING", false, "-s", subnetCIDR, "!", "-o", BridgeName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("add NAT MASQUERADE: %w", err)
	}

	if len(dropTargets) > 0 {
		// inserted at the head so DROP precedes the ACCEPTs; VM-to-gateway is INPUT, not FORWARD
		for _, dst := range dropTargets {
			if err := ensureIPTRule(ipt, "filter", "FORWARD", true, "-i", BridgeName, "-d", dst, "-m", "comment", "--comment", dropRuleComment, "-j", "DROP"); err != nil {
				return fmt.Errorf("insert FORWARD drop %s: %w", dst, err)
			}
		}
	}

	// desired rules were inserted above, so pruning the rest is gapless
	if err := reconcileDropRules(ctx, ipt, dropTargets); err != nil {
		return fmt.Errorf("reconcile drop rules: %w", err)
	}

	logger.Infof(ctx, "iptables configured for subnet %s, %d egress drop rule(s)", subnetCIDR, len(dropTargets))
	return nil
}

// resolveDropTargets canonicalizes CIDRs to iptables -S form; IPv6 is rejected, the rules go through the IPv4 binary.
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
			return nil, fmt.Errorf("parse drop CIDR %q: %w", cidr, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("drop CIDR %q must be IPv4", cidr)
		}
		out = append(out, ipNet.String())
	}
	return out, nil
}

func ensureIPTRule(ipt *iptables.IPTables, table, chain string, atHead bool, args ...string) error {
	exists, err := ipt.Exists(table, chain, args...)
	if err != nil {
		return fmt.Errorf("check rule: %w", err)
	}
	if exists {
		return nil
	}
	if atHead {
		return ipt.Insert(table, chain, 1, args...)
	}
	return ipt.Append(table, chain, args...)
}
