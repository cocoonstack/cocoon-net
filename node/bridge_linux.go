//go:build linux

package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/projecteru2/core/log"
	"github.com/vishvananda/netlink"
)

func setupBridge(ctx context.Context, gatewayIP, subnetCIDR string) error {
	logger := log.WithFunc("node.setupBridge")

	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{Name: BridgeName},
	}
	if err := netlink.LinkAdd(br); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("create bridge %s: %w", BridgeName, err)
	}

	link, err := netlink.LinkByName(BridgeName)
	if err != nil {
		return fmt.Errorf("get bridge %s: %w", BridgeName, err)
	}

	_, ipNet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return fmt.Errorf("parse subnet cidr: %w", err)
	}

	parsed := net.ParseIP(gatewayIP)
	if parsed == nil {
		return fmt.Errorf("invalid gateway ip: %s", gatewayIP)
	}
	gwIP := parsed.To4()
	if gwIP == nil {
		return fmt.Errorf("gateway is not ipv4: %s", gatewayIP)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{IP: gwIP, Mask: ipNet.Mask},
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("assign %s to %s: %w", addr.IPNet, BridgeName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring up %s: %w", BridgeName, err)
	}

	logger.Infof(ctx, "bridge %s configured with gateway %s", BridgeName, addr.IPNet)
	return nil
}
