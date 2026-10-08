package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func improveBadgeModel(t *testing.T, locale Locale, step string, iteration int) AppModel {
	t.Helper()
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeImprove); err != nil {
		t.Fatal(err)
	}
	run := domain.CeremonyRun{Definition: "axlr_improve", Version: "1.0", Instance: "axlr-m", Step: step, Iteration: iteration, Fence: "f", Repair: &domain.RepairRun{Improvement: true}}
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	m := New(Dependencies{Session: &s, Monochrome: true, Locale: locale})
	return update(m, tea.WindowSizeMsg{Width: 160, Height: 30})
}

// TestCeremonyBadgeShowsAttemptBound: a step with an attempt bound shows
// "n/3" in the footer badge; a step without one keeps the plain count.
func TestCeremonyBadgeShowsAttemptBound(t *testing.T) {
	m := improveBadgeModel(t, English, "build", 1)
	footer := ansi.Strip(m.footerView())
	if !strings.Contains(footer, "improve ceremony · build 1/3") {
		t.Fatalf("bounded badge missing: %q", footer)
	}

	m = improveBadgeModel(t, English, "revise", 1)
	footer = ansi.Strip(m.footerView())
	if !strings.Contains(footer, "revise 1") || strings.Contains(footer, "revise 1/3") {
		t.Fatalf("unbounded step changed: %q", footer)
	}

	m = improveBadgeModel(t, Spanish, "build", 2)
	footer = ansi.Strip(m.footerView())
	want := Translate(Spanish, "mode.improve") + " · build 2/3"
	if !strings.Contains(footer, "build 2/3") || !strings.Contains(footer, Translate(Spanish, "mode.improve")) {
		t.Fatalf("Spanish bounded badge missing %q: %q", want, footer)
	}
}
