package platform

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/projecteru2/core/log"
)

const (
	gceMetaBase       = "http://metadata.google.internal/computeMetadata/v1"
	gkeAliasRangeName = "cocoon-pods"
	gkePrimaryNICName = "ens4"
)

// GKEPlatform implements CloudPlatform for Google Kubernetes Engine.
type GKEPlatform struct{}

// Name returns the platform identifier.
func (g *GKEPlatform) Name() string { return "gke" }

// ProvisionNetwork configures a GCE alias IP range for the node.
//
// Steps:
//  1. Resolve instance/zone/subnet from GCE metadata.
//  2. Ensure a secondary IP range named "cocoon-pods" exists on the subnet.
//  3. Assign the alias IP range to the instance's primary NIC.
//  4. Fix GCE guest-agent route hijack (delete local route + install cron).
//  5. Return the usable IPs from the alias CIDR.
func (g *GKEPlatform) ProvisionNetwork(ctx context.Context, cfg *Config) (*NetworkResult, error) {
	logger := log.WithFunc("platform.gke.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = gkePrimaryNICName
	}

	instance, zone, project, subnet, err := gkeMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch gce metadata: %w", err)
	}
	logger.Infof(ctx, "instance=%s zone=%s project=%s subnet=%s", instance, zone, project, subnet)

	// Derive region from zone (e.g. "asia-southeast1-b" -> "asia-southeast1").
	region := zone[:strings.LastIndex(zone, "-")]

	// 1. Ensure secondary range exists on the subnet.
	if rangeErr := gkeEnsureSecondaryRange(ctx, project, region, subnet, cfg.SubnetCIDR); rangeErr != nil {
		return nil, fmt.Errorf("ensure secondary range: %w", rangeErr)
	}

	// 2. Assign alias IP to the instance.
	if aliasErr := gkeAssignAliasIP(ctx, project, zone, instance, gkeAliasRangeName, cfg.SubnetCIDR); aliasErr != nil {
		return nil, fmt.Errorf("assign alias IP: %w", aliasErr)
	}

	// 3. Fix GCE guest-agent local route hijack.
	if routeErr := gkeFixGuestAgentRoute(ctx, primaryNIC, cfg.SubnetCIDR); routeErr != nil {
		logger.Warnf(ctx, "fix guest agent route: %v", routeErr)
	}

	// 4. Compute gateway (first host IP) and IP list.
	gateway := cfg.Gateway
	if gateway == "" {
		gateway, err = firstHostIP(cfg.SubnetCIDR)
		if err != nil {
			return nil, fmt.Errorf("compute gateway: %w", err)
		}
	}

	ips, err := subnetIPs(cfg.SubnetCIDR, gateway, cfg.PoolSize)
	if err != nil {
		return nil, fmt.Errorf("compute ip list: %w", err)
	}

	return &NetworkResult{
		Platform:   g.Name(),
		SubnetCIDR: cfg.SubnetCIDR,
		Gateway:    gateway,
		IPs:        ips,
		PrimaryNIC: primaryNIC,
	}, nil
}

// Status returns current alias IP status for this instance.
func (g *GKEPlatform) Status(ctx context.Context) (*PoolStatus, error) {
	return &PoolStatus{}, nil
}

// Teardown removes the alias IP range from the instance.
func (g *GKEPlatform) Teardown(ctx context.Context) error {
	logger := log.WithFunc("platform.gke.Teardown")
	instance, zone, project, _, err := gkeMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	//nolint:gosec // constructed from metadata, not user input
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", "nic0",
		"--aliases", "",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove alias IP: %w: %s", err, out)
	}
	logger.Infof(ctx, "alias IP removed from %s", instance)
	return nil
}

// gkeMetadata fetches instance name, zone, project ID, and subnetwork name from GCE metadata.
func gkeMetadata(ctx context.Context) (instance, zone, project, subnet string, err error) {
	client := &http.Client{Timeout: 5 * time.Second}

	fetch := func(path string) (string, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet,
			gceMetaBase+path, nil)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Metadata-Flavor", "Google")
		resp, doErr := client.Do(req)
		if doErr != nil {
			return "", doErr
		}
		defer func() { _ = resp.Body.Close() }()
		b, readErr := io.ReadAll(resp.Body)
		return strings.TrimSpace(string(b)), readErr
	}

	instance, err = fetch("/instance/name")
	if err != nil {
		return "", "", "", "", fmt.Errorf("instance name: %w", err)
	}

	zoneURL, err := fetch("/instance/zone")
	if err != nil {
		return "", "", "", "", fmt.Errorf("zone: %w", err)
	}
	// zoneURL is "projects/PROJECT/zones/ZONE"
	parts := strings.Split(zoneURL, "/")
	zone = parts[len(parts)-1]
	project = parts[1]

	subnetURL, err := fetch("/instance/network-interfaces/0/subnetwork")
	if err != nil {
		return "", "", "", "", fmt.Errorf("subnetwork: %w", err)
	}
	// subnetURL is "projects/PROJECT/regions/REGION/subnetworks/SUBNET"
	subnetParts := strings.Split(subnetURL, "/")
	subnet = subnetParts[len(subnetParts)-1]

	return instance, zone, project, subnet, nil
}

// gkeEnsureSecondaryRange adds the secondary range to the subnet if absent.
func gkeEnsureSecondaryRange(ctx context.Context, project, region, subnet, cidr string) error {
	//nolint:gosec // constructed from metadata, not user input
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "networks", "subnets", "update",
		subnet,
		"--project", project,
		"--region", region,
		fmt.Sprintf("--add-secondary-ranges=%s=%s", gkeAliasRangeName, cidr),
	)
	out, err := cmd.CombinedOutput()
	// "already exists" is not a fatal error.
	if err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("update subnet: %w: %s", err, out)
	}
	return nil
}

// gkeAssignAliasIP assigns an alias IP range to the instance NIC.
func gkeAssignAliasIP(ctx context.Context, project, zone, instance, rangeName, cidr string) error {
	//nolint:gosec // constructed from metadata, not user input
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", "nic0",
		fmt.Sprintf("--aliases=%s:%s", rangeName, cidr),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("assign alias: %w: %s", err, out)
	}
	return nil
}

// gkeFixGuestAgentRoute removes the local route that the GCE guest agent installs
// for alias IP ranges (which would blackhole traffic) and installs a cron job to
// remove it on reboot.
func gkeFixGuestAgentRoute(ctx context.Context, nic, cidr string) error {
	logger := log.WithFunc("platform.gke.gkeFixGuestAgentRoute")

	// Delete local route if it exists.
	//nolint:gosec // ip args from trusted config
	del := exec.CommandContext(ctx, "ip", "route", "del",
		fmt.Sprintf("local %s dev %s table local", cidr, nic),
	)
	out, err := del.CombinedOutput()
	if err != nil {
		// Not found is fine.
		logger.Warnf(ctx, "del local route (ok if not found): %s", strings.TrimSpace(string(out)))
	}

	// Install cron job to remove the route on every reboot.
	cron := fmt.Sprintf("@reboot root ip route del local %s dev %s table local 2>/dev/null || true", cidr, nic)
	cronFile := "/etc/cron.d/cocoon-net-fix-alias"
	//nolint:gosec // bash args from trusted config
	writeCmd := exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf("echo %q > %s", cron, cronFile),
	)
	out, err = writeCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write cron job: %w: %s", err, out)
	}
	logger.Infof(ctx, "installed alias route fix cron at %s", cronFile)
	return nil
}

// firstHostIP returns the first usable host IP in the CIDR (gateway).
func firstHostIP(cidr string) (string, error) {
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

// subnetIPs returns up to count host IPs from the subnet, skipping the gateway.
func subnetIPs(cidr, gateway string, count int) ([]string, error) {
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
