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

// SubnetIPs returns up to count host IPs from the subnet, skipping
// the gateway and the broadcast address.
//
// The gateway must parse as a valid IP; an empty or unparseable value
// is an error rather than a silent "no gateway, every host is fair
// game" — the caller almost always means to reserve one. Iteration
// stops one address before the broadcast (network OR'd with the
// inverted mask) so we never hand out the broadcast as a host IP.
func SubnetIPs(cidr, gateway string, count int) ([]string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %s: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("cidr %s is not IPv4", cidr)
	}
	gwAddr, err := netip.ParseAddr(gateway)
	if err != nil {
		return nil, fmt.Errorf("parse gateway %q: %w", gateway, err)
	}
	if !gwAddr.Is4() {
		return nil, fmt.Errorf("gateway %s is not IPv4", gateway)
	}

	// Broadcast = network | ~mask. Compute it from the 4-byte form so
	// iteration stops at the last host, not the broadcast itself.
	netAddr := prefix.Masked().Addr().As4()
	bits := prefix.Bits()
	hostBits := uint32(32 - bits) //nolint:gosec // bits ∈ [0,32] from ParsePrefix
	var hostMask uint32
	if hostBits == 32 {
		hostMask = 0xFFFFFFFF
	} else {
		hostMask = (uint32(1) << hostBits) - 1
	}
	netUint := binary.BigEndian.Uint32(netAddr[:])
	var bcastBytes [4]byte
	binary.BigEndian.PutUint32(bcastBytes[:], netUint|hostMask)
	bcast := netip.AddrFrom4(bcastBytes)

	ips := make([]string, 0, count)
	for addr := prefix.Masked().Addr().Next(); prefix.Contains(addr) && len(ips) < count; addr = addr.Next() {
		if addr == gwAddr || addr == bcast {
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

// CIDRContainsCIDR reports whether outer contains inner (same network or a supernet).
func CIDRContainsCIDR(outer, inner string) (bool, error) {
	op, err := netip.ParsePrefix(outer)
	if err != nil {
		return false, fmt.Errorf("parse outer cidr %s: %w", outer, err)
	}
	ip, err := netip.ParsePrefix(inner)
	if err != nil {
		return false, fmt.Errorf("parse inner cidr %s: %w", inner, err)
	}
	if op.Bits() > ip.Bits() {
		return false, nil
	}
	return op.Contains(ip.Masked().Addr()), nil
}
