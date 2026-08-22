//go:build linux

package node

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

func setupSecondaryNICs(ifaces []string) error {
	return setupSecondaryNICsWith(ifaces, netlink.LinkByName, netlink.LinkSetUp)
}

func setupSecondaryNICsWith(ifaces []string, lookup func(string) (netlink.Link, error), setUp func(netlink.Link) error) error {
	for _, iface := range ifaces {
		link, err := lookup(iface)
		if err != nil {
			return fmt.Errorf("lookup link %s: %w", iface, err)
		}
		if link.Attrs().Flags&net.FlagUp != 0 {
			continue
		}
		if err := setUp(link); err != nil {
			return fmt.Errorf("set link %s up: %w", iface, err)
		}
	}
	return nil
}
