package platform

import (
	"fmt"
	"net"
)

// FirstHostIP returns the first usable host IP in the CIDR (typically used as gateway).
func FirstHostIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("cidr %s is not IPv4", cidr)
	}
	first := net.IP{ip[0], ip[1], ip[2], ip[3] + 1}
	return first.String(), nil
}

// SubnetIPs returns up to count host IPs from the subnet, skipping the gateway.
func SubnetIPs(cidr, gateway string, count int) ([]string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	gwIP := net.ParseIP(gateway).To4()

	var ips []string
	for ip := ip4Add(ipNet.IP.To4(), 1); ipNet.Contains(ip) && len(ips) < count; ip = ip4Add(ip, 1) {
		if ip.Equal(gwIP) {
			continue
		}
		ips = append(ips, ip.String())
	}
	return ips, nil
}

// ip4Add adds n to an IPv4 address.
func ip4Add(ip net.IP, n int) net.IP {
	result := make(net.IP, 4)
	copy(result, ip)
	val := int(result[0])<<24 | int(result[1])<<16 | int(result[2])<<8 | int(result[3])
	val += n
	result[0] = byte(val >> 24)
	result[1] = byte(val >> 16)
	result[2] = byte(val >> 8)
	result[3] = byte(val)
	return result
}
