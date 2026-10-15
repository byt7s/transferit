//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func enableConsole() {}

func restrictConfigPath(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return os.Chmod(filepath.Dir(path), 0o700)
}
