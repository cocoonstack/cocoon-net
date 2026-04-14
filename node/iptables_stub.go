//go:build !linux

package node

import (
	"context"
	"errors"
	"fmt"
)

func setupIPTables(_ context.Context, _ string, _ []string) error {
	return fmt.Errorf("iptables setup: %w", errors.ErrUnsupported)
}
