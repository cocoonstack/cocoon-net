package node

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/projecteru2/core/log"
)

const cni0Bridge = "cni0"

func setupBridge(ctx context.Context, gatewayIP, subnetCIDR string) error {
	logger := log.WithFunc("node.setupBridge")

	addCmd := exec.CommandContext(ctx, "ip", "link", "add", cni0Bridge, "type", "bridge")
	out, err := addCmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "File exists") {
		return fmt.Errorf("create bridge %s: %w: %s", cni0Bridge, err, out)
	}

	_, ipNet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return fmt.Errorf("parse subnet cidr: %w", err)
	}
	ones, _ := ipNet.Mask.Size()
	cidr := fmt.Sprintf("%s/%d", gatewayIP, ones)

	//nolint:gosec // ip args from trusted config
	addrCmd := exec.CommandContext(ctx, "ip", "addr", "replace", cidr, "dev", cni0Bridge)
	out, err = addrCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("assign %s to %s: %w: %s", cidr, cni0Bridge, err, out)
	}

	upCmd := exec.CommandContext(ctx, "ip", "link", "set", cni0Bridge, "up")
	out, err = upCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bring up %s: %w: %s", cni0Bridge, err, out)
	}

	logger.Infof(ctx, "bridge %s configured with gateway %s", cni0Bridge, cidr)
	return nil
}
