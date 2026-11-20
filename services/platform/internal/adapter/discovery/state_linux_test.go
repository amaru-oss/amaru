//go:build linux

package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStateDirectory(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv("HOME", home)
	for _, tc := range []struct{ name, xdg, want string }{
		{"absolute XDG", base, filepath.Join(base, "amaru")},
		{"relative XDG", "relative", filepath.Join(home, ".local", "state", "amaru")},
		{"unset XDG", "", filepath.Join(home, ".local", "state", "amaru")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tc.xdg)
			dir, err := StateDirectory()
			if err != nil || dir != tc.want {
				t.Fatalf("StateDirectory = %q, %v, want %q", dir, err, tc.want)
			}
			if _, err := os.Stat(filepath.Dir(dir)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("resolution created the state base")
			}
		})
	}
	t.Setenv("HOME", "relative")
	if _, err := StateDirectory(); !errors.Is(err, errUnsafe) {
		t.Fatalf("relative home: %v", err)
	}
}
