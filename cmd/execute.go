package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Execute runs the CLI for both the installed binary and go run .
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := New().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
