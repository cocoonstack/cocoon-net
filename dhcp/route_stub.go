//go:build !linux

package dhcp

import (
	"errors"
	"net"
)

func resolveLinkIndex(_ string) (int, error) {
	return 0, errors.ErrUnsupported
}

func addRoute(_ net.IP, _ int) error {
	return errors.ErrUnsupported
}

func delRoute(_ net.IP, _ int) error {
	return errors.ErrUnsupported
}
