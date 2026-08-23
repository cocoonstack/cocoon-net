//go:build linux

package node

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

func setupSecondaryNICs(ifaces []string) error {
	for _, iface := range ifaces {
		link, err := netlink.LinkByName(iface)
		if err != nil {
			return fmt.Errorf("lookup link %s: %w", iface, err)
		}
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("set link %s up: %w", iface, err)
		}
	}
	return nil
}
