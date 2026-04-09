package node

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

const cni0Bridge = "cni0"

// setupBridge creates the cni0 bridge and assigns the gateway IP.
func setupBridge(ctx context.Context, gatewayIP, subnetCIDR string) error {
	logger := log.WithFunc("node.setupBridge")

	// Create bridge if it does not exist.
	addCmd := exec.CommandContext(ctx, "ip", "link", "add", cni0Bridge, "type", "bridge")
	out, err := addCmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "File exists") {
		return fmt.Errorf("create bridge %s: %w: %s", cni0Bridge, err, out)
	}

	// Assign gateway address (idempotent via replace).
	_, mask, err := parseCIDR(subnetCIDR)
	if err != nil {
		return fmt.Errorf("parse subnet cidr: %w", err)
	}
	cidr := fmt.Sprintf("%s/%s", gatewayIP, mask)
	//nolint:gosec // ip args from trusted config
	addrCmd := exec.CommandContext(ctx, "ip", "addr", "replace", cidr, "dev", cni0Bridge)
	out, err = addrCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("assign %s to %s: %w: %s", cidr, cni0Bridge, err, out)
	}

	// Bring up the bridge.
	upCmd := exec.CommandContext(ctx, "ip", "link", "set", cni0Bridge, "up")
	out, err = upCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bring up %s: %w: %s", cni0Bridge, err, out)
	}

	logger.Infof(ctx, "bridge %s configured with gateway %s", cni0Bridge, cidr)
	return nil
}

// parseCIDR extracts the mask prefix from a CIDR string (e.g. "24" from "172.20.100.0/24").
func parseCIDR(cidr string) (network, mask string, err error) {
	parts := strings.SplitN(cidr, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid cidr: %s", cidr)
	}
	return parts[0], parts[1], nil
}
