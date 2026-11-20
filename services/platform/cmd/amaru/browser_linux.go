//go:build linux

package main

import (
	"context"
	"os/exec"
)

func openBrowser(ctx context.Context, url string) error {
	// The command receives only the expiring, single-use browser URL.
	// Child output is discarded because it may contain that URL.
	return exec.CommandContext(ctx, "xdg-open", url).Run()
}
