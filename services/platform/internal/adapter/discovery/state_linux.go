//go:build linux

package discovery

import (
	"os"
	"path/filepath"
)

// StateDirectory resolves the local Instance location without creating it.
// Both the Platform and native clients use this location.
func StateDirectory() (string, error) {
	if base := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "amaru"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", errUnsafe
	}
	return filepath.Join(home, ".local", "state", "amaru"), nil
}

// PrepareStateDirectory creates the base for Publish on a fresh installation.
// Publish owns creation and validation of the private Amaru directory itself.
func PrepareStateDirectory() (string, error) {
	dir, err := StateDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return "", errUnsafe
	}
	return dir, nil
}
