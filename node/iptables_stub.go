//go:build !linux

package node

import (
	"context"
	"errors"
)

// ClearDropRules is a no-op off Linux, where cocoon-net installs no rules.
func ClearDropRules(_ context.Context) error { return nil }

func setupIPTables(_ context.Context, _ string, _ []string, _ bool, _ []string) error {
	return errors.ErrUnsupported
}
