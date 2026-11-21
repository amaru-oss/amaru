//go:build !linux

package main

import (
	"context"
	"errors"
)

func openBrowser(context.Context, string) error {
	return errors.New("opening the default browser is unavailable on this platform")
}
