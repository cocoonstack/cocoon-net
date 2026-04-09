package node

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	dnsmasqConfDir  = "/etc/dnsmasq-cni.d"
	dnsmasqConfFile = "cni0.conf"
	dnsmasqService  = "dnsmasq-cni"
	dhcpLeaseFile   = "/var/lib/misc/dnsmasq.leases"
)

// setupDNSMasq generates /etc/dnsmasq-cni.d/cni0.conf and restarts dnsmasq-cni.
func setupDNSMasq(ctx context.Context, cfg *Config) error {
	logger := log.WithFunc("node.setupDNSMasq")

	if err := os.MkdirAll(dnsmasqConfDir, 0o750); err != nil {
		return fmt.Errorf("create dnsmasq conf dir: %w", err)
	}

	// Ensure lease file exists.
	if err := os.MkdirAll(filepath.Dir(dhcpLeaseFile), 0o750); err != nil {
		return fmt.Errorf("create lease dir: %w", err)
	}
	f, err := os.OpenFile(dhcpLeaseFile, os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // public lease file
	if err != nil {
		return fmt.Errorf("create lease file: %w", err)
	}
	_ = f.Close()

	conf, err := buildDNSMasqConf(cfg)
	if err != nil {
		return fmt.Errorf("build dnsmasq conf: %w", err)
	}
	confPath := filepath.Join(dnsmasqConfDir, dnsmasqConfFile)

	// Skip write + restart when config is unchanged.
	if existing, readErr := os.ReadFile(confPath); readErr == nil && string(existing) == conf { //nolint:gosec // known path
		logger.Info(ctx, "dnsmasq conf unchanged, skipping restart")
		return nil
	}

	if writeErr := os.WriteFile(confPath, []byte(conf), 0o644); writeErr != nil { //nolint:gosec // world-readable conf
		return fmt.Errorf("write dnsmasq conf: %w", writeErr)
	}
	logger.Infof(ctx, "wrote dnsmasq conf to %s", confPath)

	restartCmd := exec.CommandContext(ctx, "systemctl", "restart", dnsmasqService)
	out, restartErr := restartCmd.CombinedOutput()
	if restartErr != nil {
		return fmt.Errorf("restart %s: %w: %s", dnsmasqService, restartErr, out)
	}
	logger.Infof(ctx, "restarted %s", dnsmasqService)
	return nil
}

// buildDNSMasqConf generates a dnsmasq config from the Config.
// IPs are sorted numerically and grouped into contiguous ranges for efficiency.
func buildDNSMasqConf(cfg *Config) (string, error) {
	_, netMask, err := platform.CIDRMask(cfg.SubnetCIDR)
	if err != nil {
		return "", err
	}

	sorted := slices.Clone(cfg.IPs)
	platform.SortIPs(sorted)

	ranges := groupContiguous(sorted)

	var sb strings.Builder
	sb.WriteString("interface=cni0\n")
	sb.WriteString("bind-interfaces\n")
	sb.WriteString("except-interface=lo\n")
	if cfg.PrimaryNIC != "" {
		fmt.Fprintf(&sb, "except-interface=%s\n", cfg.PrimaryNIC)
	}

	for _, r := range ranges {
		fmt.Fprintf(&sb, "dhcp-range=%s,%s,%s,24h\n", r[0], r[1], netMask)
	}

	fmt.Fprintf(&sb, "dhcp-option=option:router,%s\n", cfg.Gateway)
	if len(cfg.DNSServers) > 0 {
		fmt.Fprintf(&sb, "dhcp-option=option:dns-server,%s\n", strings.Join(cfg.DNSServers, ","))
	}
	fmt.Fprintf(&sb, "dhcp-leasefile=%s\n", dhcpLeaseFile)
	sb.WriteString("dhcp-authoritative\n")
	sb.WriteString("port=0\n")
	sb.WriteString("log-dhcp\n")
	return sb.String(), nil
}

// groupContiguous groups a sorted list of IP strings into [start,end] pairs.
func groupContiguous(ips []string) [][2]string {
	if len(ips) == 0 {
		return nil
	}
	var ranges [][2]string
	start := ips[0]
	prev := platform.IP4ToUint32(ips[0])
	for _, s := range ips[1:] {
		cur := platform.IP4ToUint32(s)
		if cur == prev+1 {
			prev = cur
			continue
		}
		ranges = append(ranges, [2]string{start, platform.Uint32ToIP4(prev)})
		start = s
		prev = cur
	}
	ranges = append(ranges, [2]string{start, platform.Uint32ToIP4(prev)})
	return ranges
}
