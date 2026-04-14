//go:build !linux

package node

import (
	"context"
	"errors"
	"fmt"
)

func setupBridge(_ context.Context, _, _ string) error {
	return fmt.Errorf("bridge setup: %w", errors.ErrUnsupported)
}
