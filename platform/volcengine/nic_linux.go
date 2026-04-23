//go:build linux

package volcengine

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// bringLinkUp sets the given interface to the UP state via netlink.
// Replaces a prior `ip link set <iface> up` subprocess.
func bringLinkUp(iface string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("lookup link %s: %w", iface, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("set link %s up: %w", iface, err)
	}
	return nil
}
