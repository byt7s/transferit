//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

func restrictPath(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return os.Chmod(filepath.Dir(path), 0o700)
}
