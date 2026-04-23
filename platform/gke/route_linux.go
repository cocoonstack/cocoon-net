//go:build linux

package gke

import (
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

// delLocalAliasRoute removes the `local <cidr> dev <nic> table local` entry
// that the GCE guest agent installs for alias ranges. Equivalent to:
//
//	ip route del local <cidr> dev <nic> table local
//
// Returns nil when the route is not present (idempotent).
func delLocalAliasRoute(nic, cidr string) error {
	link, err := netlink.LinkByName(nic)
	if err != nil {
		return fmt.Errorf("lookup link %s: %w", nic, err)
	}

	_, dst, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %s: %w", cidr, err)
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       dst,
		Table:     syscall.RT_TABLE_LOCAL,
		Type:      syscall.RTN_LOCAL,
		Scope:     netlink.SCOPE_HOST,
	}
	if err := netlink.RouteDel(route); err != nil {
		return fmt.Errorf("route del local %s dev %s: %w", cidr, nic, err)
	}
	return nil
}
