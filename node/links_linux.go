//go:build linux

package node

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// PresentLinks returns the names in ifaces that exist on the host.
func PresentLinks(ifaces []string) []string {
	present := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if _, err := netlink.LinkByName(iface); err == nil {
			present = append(present, iface)
		}
	}
	return present
}

// LinkMACs returns the hardware address of each present name in ifaces, keyed by name.
func LinkMACs(ifaces []string) map[string]string {
	macs := make(map[string]string, len(ifaces))
	for _, iface := range ifaces {
		if link, err := netlink.LinkByName(iface); err == nil {
			macs[iface] = link.Attrs().HardwareAddr.String()
		}
	}
	return macs
}

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
