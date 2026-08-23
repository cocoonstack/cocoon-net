//go:build !linux

package node

import (
	"errors"
	"fmt"
)

func setupSecondaryNICs(_ []string) error {
	return fmt.Errorf("secondary NIC setup: %w", errors.ErrUnsupported)
}
