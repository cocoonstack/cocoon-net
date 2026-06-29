//go:build linux

package dhcp

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// resolveLinkIndex resolves a network interface name to its kernel index.
func resolveLinkIndex(iface string) (int, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return 0, fmt.Errorf("resolve link %s: %w", iface, err)
	}
	return link.Attrs().Index, nil
}

// addRoute adds a /32 host route for ip via the given link index.
func addRoute(ip net.IP, linkIndex int) error {
	if err := netlink.RouteReplace(hostRoute(ip, linkIndex)); err != nil {
		return fmt.Errorf("route replace %s/32: %w", ip, err)
	}
	return nil
}

// delRoute removes the /32 host route for ip.
func delRoute(ip net.IP, linkIndex int) error {
	if err := netlink.RouteDel(hostRoute(ip, linkIndex)); err != nil {
		return fmt.Errorf("route del %s/32: %w", ip, err)
	}
	return nil
}

// hostRoute builds a /32 host route for ip via the given link index.
func hostRoute(ip net.IP, linkIndex int) *netlink.Route {
	return &netlink.Route{
		Dst:       &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)},
		LinkIndex: linkIndex,
	}
}
