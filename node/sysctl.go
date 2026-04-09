package node

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/projecteru2/core/log"
)

// setupSysctl applies kernel parameters required for VPC-native VM networking.
func setupSysctl(ctx context.Context, primaryNIC string, secondaryNICs []string) error {
	logger := log.WithFunc("node.setupSysctl")

	settings := map[string]string{
		"net.ipv4.ip_forward":             "1",
		"net.ipv4.conf.all.rp_filter":     "0",
		"net.ipv4.conf.cni0.rp_filter":    "0",
		"net.ipv4.conf.default.rp_filter": "0",
	}
	if primaryNIC != "" {
		settings["net.ipv4.conf."+primaryNIC+".rp_filter"] = "0"
	}
	for _, iface := range secondaryNICs {
		settings["net.ipv4.conf."+iface+".rp_filter"] = "0"
	}

	for key, val := range settings {
		param := fmt.Sprintf("%s=%s", key, val)
		//nolint:gosec // sysctl args from trusted config
		cmd := exec.CommandContext(ctx, "sysctl", "-w", param)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("sysctl %s: %w: %s", param, err, out)
		}
	}
	logger.Info(ctx, "sysctl parameters applied")
	return nil
}
