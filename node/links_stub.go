//go:build !linux

package node

import (
	"errors"
	"fmt"
)

// PresentLinks is the identity off Linux, where nothing is probed.
func PresentLinks(ifaces []string) []string { return ifaces }

func setupSecondaryNICs(_ []string) error {
	return fmt.Errorf("secondary NIC setup: %w", errors.ErrUnsupported)
}
