//go:build !linux

package node

import (
	"context"
	"errors"
)

func setupBridge(_ context.Context, _, _ string) error {
	return errors.ErrUnsupported
}
