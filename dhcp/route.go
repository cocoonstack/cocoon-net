package dhcp

import (
	"fmt"
	"net"
	"os/exec"
)

// addRoute adds a /32 host route for ip via the given interface.
func addRoute(ip net.IP, iface string) error {
	out, err := exec.Command("ip", "route", "replace", ip.String()+"/32", "dev", iface).CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("ip route replace %s/32 dev %s: %w: %s", ip, iface, err, out)
	}
	return nil
}

// delRoute removes the /32 host route for ip.
func delRoute(ip net.IP, iface string) error {
	out, err := exec.Command("ip", "route", "del", ip.String()+"/32", "dev", iface).CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("ip route del %s/32 dev %s: %w: %s", ip, iface, err, out)
	}
	return nil
}
