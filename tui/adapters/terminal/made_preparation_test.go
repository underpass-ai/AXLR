package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type madePreparerStub struct {
	result application.MADEPreparation
	calls  int
}

func (s *madePreparerStub) Prepare(context.Context) (application.MADEPreparation, error) {
	s.calls++
	return s.result, nil
}

func TestMADEPreparationRunsFromTheMADERowOnly(t *testing.T) {
	stub := &madePreparerStub{result: application.MADEPreparation{Status: "granted", WorkIdentity: "axlr-work-1a2b", GrantID: "axlr-default-work-v1", RestartRequired: true}}
	m := sized()
	defer m.Close()
	m.deps.MADEPreparation = stub
	m.overlay = "mcp"
	m.Plugins.SetItems([]domain.PluginState{
		{Profile: domain.PluginProfile{ID: "kmp", Name: "KMP", Purpose: domain.PluginPurposeMemory, Approval: domain.ApprovalAuto}},
		{Profile: domain.PluginProfile{ID: "made", Name: "MADE", Purpose: domain.PluginPurposeCeremony, Approval: domain.ApprovalManual}},
	})
	m.Plugins.Selected = 0
	m = update(m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.overlay != "mcp" || stub.calls != 0 {
		t.Fatal("preparation offered on a non-MADE row")
	}
	m.Plugins.Selected = 1
	if !strings.Contains(ansi.Strip(m.View().Content), "P prepare for AXLR") {
		t.Fatal("MADE row does not advertise preparation")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = next.(AppModel)
	if cmd == nil || m.overlay != "made-setup" {
		t.Fatal("preparation did not start")
	}
	m = runUIOperation(m, cmd)
	view := ansi.Strip(m.View().Content)
	for _, value := range []string{"MADE for AXLR", "axlr-work-1a2b", "axlr-default-work-v1", "cannot approve human guards", "Restart AXLR"} {
		if !strings.Contains(view, value) {
			t.Fatalf("missing %q: %s", value, view)
		}
	}
	if stub.calls != 1 || m.Busy {
		t.Fatal("preparation did not finish exactly once")
	}
}
