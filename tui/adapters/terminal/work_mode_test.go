package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func send(t *testing.T, m AppModel, draft string) AppModel {
	t.Helper()
	m.Composer.Input.SetValue(draft)
	next, _ := m.Update(ControlIntent("send"))
	return next.(AppModel)
}

func TestModeCommandsSwitchPersistAndShowABadge(t *testing.T) {
	m := sized()
	defer m.Close()
	m.deps.Store = submissionStore{}
	m = send(t, m, "/escritor")
	if m.deps.Session.Mode() != domain.ModeWriter || m.Composer.Input.Value() != "" || m.Status.Error != "" {
		t.Fatalf("alias did not switch: %q %q", m.deps.Session.Mode(), m.Status.Error)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "writer mode") {
		t.Fatal("footer has no mode badge")
	}
	m = send(t, m, "/normal")
	if m.deps.Session.Mode() != domain.ModeNormal || strings.Contains(ansi.Strip(m.View().Content), "writer mode") {
		t.Fatal("normal mode still shows a badge")
	}
}

func TestModeCannotChangeDuringATurn(t *testing.T) {
	m := sized()
	defer m.Close()
	m.Busy = true
	m = send(t, m, "/review")
	if m.deps.Session.Mode() != domain.ModeNormal || m.Status.Error == "" || m.Composer.Input.Value() != "/review" {
		t.Fatal("mode changed while busy or the draft was lost")
	}
}

func TestImproveCommandAndAliasSelectTheImproveCeremony(t *testing.T) {
	for _, command := range []string{"/improve", "/mejorar"} {
		m := sized()
		m.deps.Store = submissionStore{}
		m = send(t, m, command)
		if m.deps.Session.Mode() != domain.ModeImprove || m.Status.Error != "" {
			t.Fatalf("%s: mode %q error %q", command, m.deps.Session.Mode(), m.Status.Error)
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "improve ceremony") {
			t.Fatalf("%s: footer has no improve badge", command)
		}
		m.Close()
	}
}
