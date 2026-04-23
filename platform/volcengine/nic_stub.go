//go:build !linux

package volcengine

import (
	"errors"
	"fmt"
)

func bringLinkUp(iface string) error {
	return fmt.Errorf("bring link %s up: %w", iface, errors.ErrUnsupported)
}
