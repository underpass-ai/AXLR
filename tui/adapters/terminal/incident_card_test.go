package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type draftFiles map[string][]byte

func (f draftFiles) Read(_ context.Context, path string, _ int) ([]byte, bool, error) {
	content, ok := f[path]
	return content, ok, nil
}
func (draftFiles) Write(context.Context, string, []byte) error { return nil }
func (draftFiles) MakeDir(context.Context, string) error       { return nil }

func awaitingModel(t *testing.T, returns int) AppModel {
	t.Helper()
	draft := []byte("# Postmortem checkout\nSin culpables.")
	sum := sha256.Sum256(draft)
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeIncident); err != nil {
		t.Fatal(err)
	}
	incident := &domain.IncidentRun{Slug: "checkout", DraftPath: "docs/incidents/checkout.draft.md", DraftDigest: hex.EncodeToString(sum[:]), Returns: returns, Awaiting: domain.AwaitingApproval}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_incident", Version: "1.0", Instance: "axlr-i", Step: "present", Iteration: 1, Fence: "f1", Incident: incident}); err != nil {
		t.Fatal(err)
	}
	driver := &application.CeremonyDriver{Files: draftFiles{"docs/incidents/checkout.draft.md": draft}}
	m := New(Dependencies{Session: &s, Start: application.StartTurnUseCase{Continue: application.ContinueTurnUseCase{Ceremonies: driver}}, Monochrome: true, Locale: Spanish})
	return update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestTheApprovalCardOpensOnLoadWithTheExactDraft(t *testing.T) {
	m := awaitingModel(t, 0)
	if m.overlay != "incident" {
		t.Fatalf("card not open on load: %q", m.overlay)
	}
	view := m.View().Content
	for _, want := range []string{"Postmortem de la incidencia", "a aprobar · d devolver", "Sin culpables", "Devoluciones restantes: 2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("card lacks %q:\n%s", want, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != "" {
		t.Fatal("esc did not close the card")
	}
	if m = m.autoOpenIncidentCard(); m.overlay != "" {
		t.Fatal("a closed card reopened on its own for the same draft")
	}
}

func TestSendingBackNeedsAReasonAndStopsAfterTwoReturns(t *testing.T) {
	m := awaitingModel(t, 0)
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !m.IncidentCard.Reasoning {
		t.Fatal("d did not ask for a reason")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Status.Error != Translate(Spanish, "incident.reasonRequired") || m.overlay != "incident" {
		t.Fatalf("empty reason accepted: %q", m.Status.Error)
	}
	spent := awaitingModel(t, domain.MaxIncidentReturns)
	if strings.Contains(spent.View().Content, "d devolver") {
		t.Fatal("d offered after two returns")
	}
	spent = update(spent, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if spent.IncidentCard.Reasoning {
		t.Fatal("d accepted after two returns")
	}
}
