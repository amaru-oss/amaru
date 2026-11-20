package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stderr, openBrowser)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stderr io.Writer, open func(context.Context, string) error) int {
	if len(args) != 1 || args[0] != "open" {
		fmt.Fprintln(stderr, "Usage: amaru open")
		return 2
	}
	if err := openInstance(ctx, open); err != nil {
		fmt.Fprintln(stderr, "amaru:", err)
		return 1
	}
	return 0
}
