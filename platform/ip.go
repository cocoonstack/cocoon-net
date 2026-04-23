package platform

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"slices"
)

// IP4ToUint32 converts an IPv4 string to its uint32 representation.
func IP4ToUint32(s string) uint32 {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return 0
	}
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:])
}

// Uint32ToIP4 converts a uint32 to a dotted-decimal IPv4 string.
func Uint32ToIP4(n uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return netip.AddrFrom4(b).String()
}

// SortIPs sorts IPv4 address strings numerically in place.
func SortIPs(ips []string) {
	slices.SortFunc(ips, func(a, b string) int {
		return cmp.Compare(IP4ToUint32(a), IP4ToUint32(b))
	})
}

// FirstHostIP returns the first usable host IP in the CIDR (typically used as gateway).
func FirstHostIP(cidr string) (string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return "", fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("cidr %s is not IPv4", cidr)
	}
	// prefix.Masked() gives the network address; .Next() increments IPv4 with
	// proper carry, so we never hit the "last octet overflow" bug that a naive
	// byte increment (e.g. ip[3]+1) would produce when the network ends in .255.
	first := prefix.Masked().Addr().Next()
	if !first.IsValid() {
		return "", fmt.Errorf("cidr %s has no host IPs", cidr)
	}
	return first.String(), nil
}

// SubnetIPs returns up to count host IPs from the subnet, skipping the gateway.
func SubnetIPs(cidr, gateway string, count int) ([]string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	gwAddr, _ := netip.ParseAddr(gateway)

	ips := make([]string, 0, count)
	for addr := prefix.Masked().Addr().Next(); prefix.Contains(addr) && len(ips) < count; addr = addr.Next() {
		if addr == gwAddr {
			continue
		}
		ips = append(ips, addr.String())
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
