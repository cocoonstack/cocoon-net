package node

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/projecteru2/core/log"
)

const (
	dnsmasqConfDir  = "/etc/dnsmasq-cni.d"
	dnsmasqConfFile = "cni0.conf"
	dnsmasqService  = "dnsmasq-cni"
	dhcpLeaseFile   = "/var/lib/misc/dnsmasq.leases"
)

// setupDNSMasq generates /etc/dnsmasq-cni.d/cni0.conf and restarts dnsmasq-cni.
func setupDNSMasq(ctx context.Context, gateway, subnetCIDR string, ips, dnsServers []string, excludeIface string) error {
	logger := log.WithFunc("node.setupDNSMasq")

	if err := os.MkdirAll(dnsmasqConfDir, 0o750); err != nil {
		return fmt.Errorf("create dnsmasq conf dir: %w", err)
	}

	conf := buildDNSMasqConf(gateway, subnetCIDR, ips, dnsServers, excludeIface)
	confPath := filepath.Join(dnsmasqConfDir, dnsmasqConfFile)
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil { //nolint:gosec // world-readable conf
		return fmt.Errorf("write dnsmasq conf: %w", err)
	}
	logger.Infof(ctx, "wrote dnsmasq conf to %s", confPath)

	// Ensure lease file exists.
	if err := os.MkdirAll(filepath.Dir(dhcpLeaseFile), 0o750); err != nil {
		return fmt.Errorf("create lease dir: %w", err)
	}
	if _, err := os.Stat(dhcpLeaseFile); os.IsNotExist(err) {
		if err := os.WriteFile(dhcpLeaseFile, nil, 0o644); err != nil { //nolint:gosec // public lease file
			return fmt.Errorf("create lease file: %w", err)
		}
	}

	// Restart service.
	restartCmd := exec.CommandContext(ctx, "systemctl", "restart", dnsmasqService)
	out, err := restartCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", dnsmasqService, err, out)
	}
	logger.Infof(ctx, "restarted %s", dnsmasqService)
	return nil
}

// buildDNSMasqConf generates a dnsmasq config from the IP list.
// IPs are sorted numerically and grouped into contiguous ranges for efficiency.
func buildDNSMasqConf(gateway, subnetCIDR string, ips, dnsServers []string, excludeIface string) string {
	_, netMask := cidrMask(subnetCIDR)

	sorted := make([]string, len(ips))
	copy(sorted, ips)
	sort.Slice(sorted, func(i, j int) bool {
		return ip4ToUint32(sorted[i]) < ip4ToUint32(sorted[j])
	})

	ranges := groupContiguous(sorted)

	var sb strings.Builder
	sb.WriteString("interface=cni0\n")
	sb.WriteString("bind-interfaces\n")
	sb.WriteString("except-interface=lo\n")
	if excludeIface != "" {
		fmt.Fprintf(&sb, "except-interface=%s\n", excludeIface)
	}

	for _, r := range ranges {
		fmt.Fprintf(&sb, "dhcp-range=%s,%s,%s,24h\n", r[0], r[1], netMask)
	}

	fmt.Fprintf(&sb, "dhcp-option=option:router,%s\n", gateway)
	if len(dnsServers) > 0 {
		fmt.Fprintf(&sb, "dhcp-option=option:dns-server,%s\n", strings.Join(dnsServers, ","))
	}
	fmt.Fprintf(&sb, "dhcp-leasefile=%s\n", dhcpLeaseFile)
	sb.WriteString("dhcp-authoritative\n")
	sb.WriteString("port=0\n")
	sb.WriteString("log-dhcp\n")
	return sb.String()
}

// groupContiguous groups a sorted list of IP strings into [start,end] pairs.
func groupContiguous(ips []string) [][2]string {
	if len(ips) == 0 {
		return nil
	}
	var ranges [][2]string
	start := ips[0]
	prev := ip4ToUint32(ips[0])
	for _, s := range ips[1:] {
		cur := ip4ToUint32(s)
		if cur == prev+1 {
			prev = cur
			continue
		}
		ranges = append(ranges, [2]string{start, uint32ToIP4(prev)})
		start = s
		prev = cur
	}
	ranges = append(ranges, [2]string{start, uint32ToIP4(prev)})
	return ranges
}

// cidrMask returns the network address and dotted-decimal mask for a CIDR.
func cidrMask(cidr string) (network, mask string) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "255.255.255.0"
	}
	m := ipNet.Mask
	return ipNet.IP.String(), fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}

// ip4ToUint32 converts an IPv4 string to uint32.
func ip4ToUint32(s string) uint32 {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// uint32ToIP4 converts a uint32 to dotted-decimal IPv4.
func uint32ToIP4(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", n>>24, (n>>16)&0xFF, (n>>8)&0xFF, n&0xFF)
}
