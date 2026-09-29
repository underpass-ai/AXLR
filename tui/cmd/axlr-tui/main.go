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
	code := run(ctx, os.Args[1:], os.Getenv, func(model tea.Model) error { _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); return err }, os.Stderr)
	stop()
	os.Exit(code)
}
