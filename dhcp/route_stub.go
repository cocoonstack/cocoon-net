//go:build !linux

package dhcp

import (
	"errors"
	"fmt"
	"net"
)

func resolveLinkIndex(_ string) (int, error) {
	return 0, fmt.Errorf("resolve link: %w", errors.ErrUnsupported)
}

func addRoute(ip net.IP, _ int) error {
	return fmt.Errorf("add route %s: %w", ip, errors.ErrUnsupported)
}

func delRoute(ip net.IP, _ int) error {
	return fmt.Errorf("del route %s: %w", ip, errors.ErrUnsupported)
}
