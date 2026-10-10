// Command otelplan resolves instrumentation policies and builds isolated applications.
// It cancels CLI operations on interrupt or termination signals.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/DiLRandI/OTelPlan/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.RunWithInput(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}
