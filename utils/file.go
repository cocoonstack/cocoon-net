// Package utils holds helpers shared across cocoon-net packages.
package utils

import (
	"fmt"
	"os"
)

// WriteFileAtomic writes data to path via a sibling tmp file and rename, so a crash never leaves a partial file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	// WriteFile applies perm only on create; a leftover tmp or the umask would otherwise leak through
	if err := os.Chmod(tmp, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}
