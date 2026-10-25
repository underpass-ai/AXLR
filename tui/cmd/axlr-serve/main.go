package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/underpass-ai/AXLR/adapters/local"
)

func main() {
	// Inside an execution's sandbox, this binary is the egress relay.
	if len(os.Args) > 1 && os.Args[1] == local.EgressRelayCommand {
		os.Exit(local.RunEgressRelay(os.Args[2:]))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
