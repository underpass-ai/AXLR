package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

type readyEngine struct{ err error }

func (e readyEngine) Ready(context.Context, string, string) error { return e.err }
func (readyEngine) Start(context.Context, string, string, string, map[string]string) error {
	return nil
}
func (readyEngine) Claim(context.Context, string, string, string, time.Duration) (string, error) {
	return "", nil
}
func (readyEngine) Complete(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (readyEngine) Transition(context.Context, string, string) (string, error) { return "", nil }
func (readyEngine) Inspect(context.Context, string) (application.CeremonyView, error) {
	return application.CeremonyView{}, nil
}
func (readyEngine) Cancel(context.Context, string, string) error { return nil }

type preparerStub struct {
	result application.MADEPreparation
	err    error
	calls  int
}

func (p *preparerStub) Prepare(context.Context) (application.MADEPreparation, error) {
	p.calls++
	return p.result, p.err
}

// A store that lacks axlr_session gets it published at start, through the
// preparer P uses; a store that has it is left alone.
func TestPrepareSessionDefinitionPublishesOnlyWhenMissing(t *testing.T) {
	preparer := &preparerStub{result: application.MADEPreparation{Status: "ready", Published: []string{"axlr_session 1.0"}}}
	if line := prepareSessionDefinition(context.Background(), readyEngine{}, preparer); line != "" || preparer.calls != 0 {
		t.Fatalf("%q %d", line, preparer.calls)
	}
	missing := readyEngine{err: application.NotPreparedError(application.MissingDefinition{Name: "axlr_session", Version: "1.0"})}
	if line := prepareSessionDefinition(context.Background(), missing, preparer); !strings.Contains(line, "published axlr_session 1.0") || preparer.calls != 1 {
		t.Fatalf("%q %d", line, preparer.calls)
	}
	preparer.result.Published = nil
	if line := prepareSessionDefinition(context.Background(), missing, preparer); !strings.Contains(line, "press p") {
		t.Fatalf("%q", line)
	}
	preparer.err = errors.New("grant conflict")
	if line := prepareSessionDefinition(context.Background(), missing, preparer); !strings.Contains(line, "grant conflict") {
		t.Fatalf("%q", line)
	}
	if line := prepareSessionDefinition(context.Background(), readyEngine{err: errors.New("connection reset")}, preparer); !strings.Contains(line, "connection reset") {
		t.Fatalf("%q", line)
	}
	if line := prepareSessionDefinition(context.Background(), nil, preparer); line != "" {
		t.Fatalf("%q", line)
	}
}
