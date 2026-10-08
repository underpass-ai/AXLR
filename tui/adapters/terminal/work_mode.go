package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// slashModes maps a mode command to the mode it selects.
var slashModes = map[string]domain.WorkMode{
	"/normal": domain.ModeNormal, "/review": domain.ModeReview, "/writer": domain.ModeWriter, "/research": domain.ModeResearch,
	"/debug": domain.ModeDebug, "/delivery": domain.ModeDelivery, "/incident": domain.ModeIncident, "/repair": domain.ModeRepair,
	"/improve": domain.ModeImprove, "/plan": domain.ModePlan,
}

// switchMode changes how the next turn works and saves it with the session.
// The draft stays when the change is refused, so nothing typed is lost.
func (m AppModel) switchMode(mode domain.WorkMode) (AppModel, tea.Cmd) {
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	if m.deps.Session == nil || m.deps.Store == nil {
		m.Status.Error = m.Theme.T("mode.unavailable")
		return m, nil
	}
	next := *m.deps.Session
	if err := next.SetMode(mode); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	if err := m.deps.Store.Save(m.lifetime.ctx, next); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	*m.deps.Session = next
	m.Header.State = next.Export()
	m.Status.Error = ""
	m.Composer.Input.Reset()
	return m, nil
}

// stopCeremony is /stop-ceremony: the person ends the live ceremony.
func (m AppModel) stopCeremony() (AppModel, tea.Cmd) {
	if m.Busy {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	if m.deps.Session == nil {
		m.Status.Error = m.Theme.T("ceremony.nothingToStop")
		return m, nil
	}
	if _, live := m.deps.Session.Ceremony(); !live {
		m.Status.Error = m.Theme.T("ceremony.nothingToStop")
		return m, nil
	}
	start := m.deps.Start
	m.Status.Error = ""
	return m, m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		return start.StopCeremony(ctx, s, emit)
	})
}
