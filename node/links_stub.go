//go:build !linux

package node

import "errors"

// PresentLinks is the identity off Linux, where nothing is probed.
func PresentLinks(ifaces []string) []string { return ifaces }

func setupSecondaryNICs(_ []string) error {
	return errors.ErrUnsupported
}
