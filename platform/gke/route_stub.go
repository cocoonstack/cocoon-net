//go:build !linux

package gke

import "errors"

func delLocalAliasRoute(_, _ string) error {
	return errors.ErrUnsupported
}
