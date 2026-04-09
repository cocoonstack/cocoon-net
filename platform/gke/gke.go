package gke

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/projecteru2/core/log"

	"github.com/cocoonstack/cocoon-net/platform"
)

var _ platform.CloudPlatform = (*GKE)(nil)

const (
	metaBase       = "http://metadata.google.internal/computeMetadata/v1"
	aliasRangeName = "cocoon-pods"

	// DefaultNIC is the default primary NIC on GKE nodes.
	DefaultNIC = "ens4"

	detectionURL     = metaBase + "/instance/zone"
	detectionTimeout = 2 * time.Second
	metadataTimeout  = 5 * time.Second
)

// GKE implements CloudPlatform for Google Kubernetes Engine.
type GKE struct{}

// Detect probes the GCE metadata endpoint to determine if running on GKE.
func Detect(ctx context.Context) bool {
	client := &http.Client{Timeout: detectionTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detectionURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Name returns the platform identifier.
func (g *GKE) Name() string { return platform.PlatformGKE }

// ProvisionNetwork configures a GCE alias IP range for the node.
func (g *GKE) ProvisionNetwork(ctx context.Context, cfg *platform.Config) (*platform.NetworkResult, error) {
	logger := log.WithFunc("gke.ProvisionNetwork")

	primaryNIC := cfg.PrimaryNIC
	if primaryNIC == "" {
		primaryNIC = DefaultNIC
	}

	instance, zone, project, subnet, err := fetchMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch gce metadata: %w", err)
	}
	logger.Infof(ctx, "instance=%s zone=%s project=%s subnet=%s", instance, zone, project, subnet)

	region := zone[:strings.LastIndex(zone, "-")]

	if err = ensureSecondaryRange(ctx, project, region, subnet, cfg.SubnetCIDR); err != nil {
		return nil, fmt.Errorf("ensure secondary range: %w", err)
	}

	if err = assignAliasIP(ctx, project, zone, instance, cfg.SubnetCIDR); err != nil {
		return nil, fmt.Errorf("assign alias IP: %w", err)
	}

	if err = fixGuestAgentRoute(ctx, primaryNIC, cfg.SubnetCIDR); err != nil {
		logger.Warnf(ctx, "fix guest agent route: %v", err)
	}

	gateway := cfg.Gateway
	if gateway == "" {
		gateway, err = platform.FirstHostIP(cfg.SubnetCIDR)
		if err != nil {
			return nil, fmt.Errorf("compute gateway: %w", err)
		}
	}

	ips, err := platform.SubnetIPs(cfg.SubnetCIDR, gateway, cfg.PoolSize)
	if err != nil {
		return nil, fmt.Errorf("compute ip list: %w", err)
	}

	return &platform.NetworkResult{
		Platform:   g.Name(),
		SubnetCIDR: cfg.SubnetCIDR,
		Gateway:    gateway,
		IPs:        ips,
		PrimaryNIC: primaryNIC,
	}, nil
}

// Status returns current alias IP status for this instance.
func (g *GKE) Status(_ context.Context) (*platform.PoolStatus, error) {
	return &platform.PoolStatus{}, nil
}

// Teardown removes the alias IP range from the instance.
func (g *GKE) Teardown(ctx context.Context) error {
	logger := log.WithFunc("gke.Teardown")
	instance, zone, project, _, err := fetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetch gce metadata: %w", err)
	}

	//nolint:gosec // args constructed from GCE metadata
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

// fetchMetadata retrieves instance name, zone, project ID, and subnetwork name from GCE metadata.
func fetchMetadata(ctx context.Context) (instance, zone, project, subnet string, err error) {
	client := &http.Client{Timeout: metadataTimeout}

	fetch := func(path string) (string, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, metaBase+path, nil)
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
	// zoneURL format: "projects/PROJECT/zones/ZONE"
	parts := strings.Split(zoneURL, "/")
	zone = parts[len(parts)-1]
	project = parts[1]

	subnetURL, err := fetch("/instance/network-interfaces/0/subnetwork")
	if err != nil {
		return "", "", "", "", fmt.Errorf("subnetwork: %w", err)
	}
	// subnetURL format: "projects/PROJECT/regions/REGION/subnetworks/SUBNET"
	subnetParts := strings.Split(subnetURL, "/")
	subnet = subnetParts[len(subnetParts)-1]

	return instance, zone, project, subnet, nil
}

func ensureSecondaryRange(ctx context.Context, project, region, subnet, cidr string) error {
	//nolint:gosec // args constructed from GCE metadata
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "networks", "subnets", "update",
		subnet,
		"--project", project,
		"--region", region,
		fmt.Sprintf("--add-secondary-ranges=%s=%s", aliasRangeName, cidr),
	)
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("update subnet: %w: %s", err, out)
	}
	return nil
}

func assignAliasIP(ctx context.Context, project, zone, instance, cidr string) error {
	//nolint:gosec // args constructed from GCE metadata
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", "nic0",
		fmt.Sprintf("--aliases=%s:%s", aliasRangeName, cidr),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("assign alias: %w: %s", err, out)
	}
	return nil
}

func fixGuestAgentRoute(ctx context.Context, nic, cidr string) error {
	logger := log.WithFunc("gke.fixGuestAgentRoute")

	//nolint:gosec // args from trusted config
	del := exec.CommandContext(ctx, "ip", "route", "del",
		"local", cidr, "dev", nic, "table", "local",
	)
	out, err := del.CombinedOutput()
	if err != nil {
		logger.Warnf(ctx, "del local route (ok if not found): %s", strings.TrimSpace(string(out)))
	}

	cron := fmt.Sprintf("@reboot root ip route del local %s dev %s table local 2>/dev/null || true", cidr, nic)
	cronFile := "/etc/cron.d/cocoon-net-fix-alias"
	//nolint:gosec // args from trusted config
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
