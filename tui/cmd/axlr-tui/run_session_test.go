package main

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
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
func (readyEngine) Report(context.Context, application.HostReport) error {
	return nil
}
func (readyEngine) Handoff(context.Context, application.HostHandoff) error {
	return nil
}
func (readyEngine) Renew(context.Context, string, string, string, time.Duration) error {
	return nil
}
func (readyEngine) Operation(context.Context, string, string, string) (application.ClaimOperation, error) {
	return application.ClaimOperation{}, nil
}

type preparerStub struct {
	result application.OrchestratorPreparation
	err    error
	calls  int
}

func (p *preparerStub) Prepare(context.Context) (application.OrchestratorPreparation, error) {
	p.calls++
	return p.result, p.err
}

func (p *preparerStub) Publish(context.Context, string) (string, error) { return "", p.err }

// A store that lacks axlr_session gets it published at start, through the
// preparer P uses; a store that has it is left alone.
func TestPrepareSessionDefinitionPublishesOnlyWhenMissing(t *testing.T) {
	preparer := &preparerStub{result: application.OrchestratorPreparation{Status: "ready", Published: []string{"axlr_session 1.0"}}}
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

func TestMissingEnginesNamesWhatIsNotConnected(t *testing.T) {
	manifest := func(id string) plugins.Registration {
		return plugins.Registration{Manifest: plugins.Manifest{ID: root.PluginID(id), Command: "/bin/true"}}
	}
	if got := missingEngines(nil); len(got) != 2 || got[0] != "KMP" || got[1] != "MADE" {
		t.Fatalf("%v", got)
	}
	if got := missingEngines([]plugins.Registration{manifest("kmp")}); len(got) != 1 || got[0] != "MADE" {
		t.Fatalf("%v", got)
	}
	if got := missingEngines([]plugins.Registration{manifest("kmp"), manifest("made")}); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}
