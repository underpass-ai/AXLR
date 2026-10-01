package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
)

type engineUpdaterStub struct {
	results []application.EngineUpdateResult
	err     error
	calls   int
}

func (s *engineUpdaterStub) Update(context.Context) ([]application.EngineUpdateResult, error) {
	s.calls++
	return s.results, s.err
}

func TestUpdateIsLocalAndReportsBothEngines(t *testing.T) {
	stub := &engineUpdaterStub{results: []application.EngineUpdateResult{
		{Engine: "made", PreviousVersion: "0.7.8", Version: "0.9.1", Status: "updated", RestartRequired: true},
		{Engine: "kmp", Version: "0.24.0", Status: "current"},
	}}
	m := sized()
	defer m.Close()
	m.deps.EngineUpdates = stub
	m.Composer.Input.SetValue("/update")
	before := len(m.Header.State.Messages)
	next, cmd := m.Update(ControlIntent("send"))
	m = next.(AppModel)
	if cmd == nil || !m.Busy || m.overlay != "updates" || m.Composer.Input.Value() != "" {
		t.Fatal("update did not start locally")
	}
	if !strings.Contains(m.View().Content, "Checking the official") {
		t.Fatal("missing progress")
	}
	m = runUIOperation(m, cmd)
	if stub.calls != 1 || m.Busy || len(m.Header.State.Messages) != before {
		t.Fatal("update altered conversation or remained busy")
	}
	view := ansi.Strip(m.View().Content)
	for _, value := range []string{"MADE", "0.7.8 → 0.9.1", "KMP", "0.24.0", "Restart AXLR"} {
		if !strings.Contains(view, value) {
			t.Fatalf("missing %s: %s", value, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "" {
		t.Fatal("results could not be closed")
	}
}

func TestUpdatePreservesDraftWhenBusyOrUnavailable(t *testing.T) {
	for _, busy := range []bool{false, true} {
		m := sized()
		defer m.Close()
		m.Busy = busy
		m.Composer.Input.SetValue("/update")
		if busy {
			m.deps.EngineUpdates = &engineUpdaterStub{}
		}
		next, cmd := m.Update(ControlIntent("send"))
		m = next.(AppModel)
		if cmd != nil || m.Composer.Input.Value() != "/update" || m.Status.Error == "" {
			t.Fatal("blocked update lost its draft")
		}
	}
}

func TestUpdatePaletteAndLocalizedNarrowResults(t *testing.T) {
	stub := &engineUpdaterStub{results: []application.EngineUpdateResult{{Engine: "made", Status: "failed", Error: "checksum mismatch\x1b]52;c;injection\a"}, {Engine: "kmp", Status: "skipped"}}}
	m := New(Dependencies{EngineUpdates: stub, Locale: Spanish})
	defer m.Close()
	m = update(m, tea.WindowSizeMsg{Width: 55, Height: 18})
	m = update(m, ControlIntent("palette"))
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = runUIOperation(next.(AppModel), cmd)
	view := ansi.Strip(m.View().Content)
	if stub.calls != 1 || !strings.Contains(view, "No se pudo actualizar") || !strings.Contains(view, "KMP") || strings.Contains(view, "injection") {
		t.Fatalf("bad result: %s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 55 {
			t.Fatal("overlay overflow")
		}
	}
	text := engineUpdateContent(nil, errors.New("cancelled"), m.Theme)
	if !strings.Contains(text, "cancelled") || strings.Contains(text, "Comprobación terminada") {
		t.Fatal("failure reported as complete")
	}
}
