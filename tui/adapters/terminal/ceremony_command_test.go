package terminal

import (
	"context"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type definitionsStub struct {
	listed []application.CeremonyDefinitionSummary
}

func (d definitionsStub) Describe(context.Context, string, string) (application.CeremonyDefinition, error) {
	return application.CeremonyDefinition{}, nil
}
func (d definitionsStub) List(context.Context) ([]application.CeremonyDefinitionSummary, error) {
	return d.listed, nil
}

// /ceremony reads MADE in an operation, not on the screen's thread (review
// of 14 October 2026: up to 15 s frozen), then fills the panel or arms the
// definition; a missing version names the published ones.
func TestCeremonyListsInAnOperationAndArms(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	defer m.zones.Close()
	m.deps.Definitions = definitionsStub{listed: []application.CeremonyDefinitionSummary{{Name: "axlr_notes", Version: "1.0", Description: "notes", Steps: 2}, {Name: "axlr_notes", Version: "1.1", Steps: 2}}}
	next, cmd := m.ceremonyCommand("")
	m = next.(AppModel)
	if !m.Busy || m.overlay != "ceremonies" || !strings.Contains(m.Info.Text(), m.Theme.T("ceremonies.loading")) {
		t.Fatalf("busy %v overlay %q panel %q", m.Busy, m.overlay, m.Info.Text())
	}
	m = drain(t, m, cmd)
	if m.Busy || !strings.Contains(m.Info.Text(), "axlr_notes 1.1") {
		t.Fatalf("panel %q", m.Info.Text())
	}
	m.overlay = ""
	next, cmd = m.ceremonyCommand("axlr_notes 2.0")
	m = drain(t, next.(AppModel), cmd)
	if !strings.Contains(m.Status.Error, "1.0, 1.1") {
		t.Fatalf("unknown version: %q", m.Status.Error)
	}
	next, cmd = m.ceremonyCommand("axlr_notes")
	m = drain(t, next.(AppModel), cmd)
	if state := m.deps.Session.Export(); state.Mode != domain.ModeCeremony || state.ChosenCeremony.Version != "1.1" {
		t.Fatalf("armed %+v, error %q", state.ChosenCeremony, m.Status.Error)
	}
}
