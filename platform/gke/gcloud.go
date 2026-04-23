package gke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

const (
	filePerm    = 0o644
	cronFixFile = "/etc/cron.d/cocoon-net-fix-alias"
)

// gcloudRun executes the `gcloud` CLI. Every invocation is a tech-debt
// hotspot documented at package level — see gke.go. Each call is logged
// at debug level so operators can correlate slowness with external binary
// spawns.
func gcloudRun(ctx context.Context, args ...string) ([]byte, error) {
	logger := log.WithFunc("gke.gcloudRun")
	logger.Debugf(ctx, "spawn external binary: gcloud %s", strings.Join(args, " "))

	//nolint:gosec // args from metadata / constants
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("gcloud %s: %w: %s", strings.Join(args[:min(3, len(args))], " "), err, out)
	}
	return out, nil
}

func ensureSecondaryRange(ctx context.Context, project, region, subnet, cidr string) error {
	out, err := gcloudRun(ctx,
		"compute", "networks", "subnets", "update",
		subnet,
		"--project", project,
		"--region", region,
		fmt.Sprintf("--add-secondary-ranges=%s=%s", aliasRangeName, cidr),
	)
	if err != nil && !strings.Contains(string(out), "already exists") {
		return fmt.Errorf("update subnet: %w", err)
	}
	return nil
}

func assignAliasIP(ctx context.Context, project, zone, instance, cidr string) error {
	_, err := gcloudRun(ctx,
		"compute", "instances", "network-interfaces", "update",
		instance,
		"--project", project,
		"--zone", zone,
		"--network-interface", "nic0",
		fmt.Sprintf("--aliases=%s:%s", aliasRangeName, cidr),
	)
	if err != nil {
		return fmt.Errorf("assign alias: %w", err)
	}
	return nil
}

// fixGuestAgentRoute removes the local route the GCE guest agent auto-installs
// for alias ranges (which would otherwise black-hole traffic back to the VM)
// and persists a reboot cron entry to re-apply the fix.
func fixGuestAgentRoute(ctx context.Context, nic, cidr string) error {
	logger := log.WithFunc("gke.fixGuestAgentRoute")
	logger.Debugf(ctx, "spawn external binary: ip route del local %s dev %s table local", cidr, nic)

	//nolint:gosec // args from trusted config
	del := exec.CommandContext(ctx, "ip", "route", "del",
		"local", cidr, "dev", nic, "table", "local",
	)
	out, err := del.CombinedOutput()
	if err != nil {
		logger.Warnf(ctx, "del local route (ok if not found): %s", strings.TrimSpace(string(out)))
	}

	cron := fmt.Sprintf("@reboot root ip route del local %s dev %s table local 2>/dev/null || true\n", cidr, nic)
	if err := os.WriteFile(cronFixFile, []byte(cron), filePerm); err != nil {
		return fmt.Errorf("write cron job %s: %w", cronFixFile, err)
	}
	logger.Infof(ctx, "installed alias route fix cron at %s", cronFixFile)
	return nil
}
