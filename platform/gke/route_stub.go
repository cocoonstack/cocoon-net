//go:build !linux

package gke

import (
	"errors"
	"fmt"
)

func delLocalAliasRoute(nic, cidr string) error {
	return fmt.Errorf("del local alias route %s dev %s: %w", cidr, nic, errors.ErrUnsupported)
}
