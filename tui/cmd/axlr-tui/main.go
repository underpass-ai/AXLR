package main

import (
	tea "charm.land/bubbletea/v2"
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] == logsCommand {
		os.Exit(runLogs(os.Args[2:], os.Getenv, os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == setupCommand {
		os.Exit(runSetupCommand(ctx, os.Args[2:], os.Getenv, os.Stdin, os.Stderr))
	}
	code := run(ctx, os.Args[1:], os.Getenv, func(model tea.Model) error { _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); return err }, os.Stderr)
	stop()
	os.Exit(code)
}
