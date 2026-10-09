package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] == logsCommand {
		os.Exit(runLogs(os.Args[2:], os.Getenv, os.Stdout, os.Stderr))
	}
	code := run(ctx, os.Args[1:], os.Getenv, func(model tea.Model) error { _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); return err }, os.Stderr)
	stop()
	os.Exit(code)
}
