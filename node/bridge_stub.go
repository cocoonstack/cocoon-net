//go:build !linux

package node

import (
	"context"
	"errors"
)

func setupBridge(_ context.Context, _, _ string, _ int) error {
	return errors.ErrUnsupported
}
