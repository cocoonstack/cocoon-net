package platform

import (
	"cmp"
	"fmt"
	"net"
	"slices"
)

// IP4ToUint32 converts an IPv4 string to its uint32 representation.
func IP4ToUint32(s string) uint32 {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// Uint32ToIP4 converts a uint32 to a dotted-decimal IPv4 string.
func Uint32ToIP4(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", n>>24, (n>>16)&0xFF, (n>>8)&0xFF, n&0xFF)
}

// SortIPs sorts IPv4 address strings numerically in place.
func SortIPs(ips []string) {
	slices.SortFunc(ips, func(a, b string) int {
		return cmp.Compare(IP4ToUint32(a), IP4ToUint32(b))
	})
}

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

	ips := make([]string, 0, count)
	for ip := ip4Add(ipNet.IP.To4(), 1); ipNet.Contains(ip) && len(ips) < count; ip = ip4Add(ip, 1) {
		if ip.Equal(gwIP) {
			continue
		}
		ips = append(ips, ip.String())
	}
	return ips, nil
}

// CIDRMask returns the network address and dotted-decimal mask for a CIDR.
func CIDRMask(cidr string) (network, mask string, err error) {
	_, ipNet, parseErr := net.ParseCIDR(cidr)
	if parseErr != nil {
		return "", "", fmt.Errorf("parse cidr %s: %w", cidr, parseErr)
	}
	m := ipNet.Mask
	return ipNet.IP.String(), fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3]), nil
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
