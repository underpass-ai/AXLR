package terminal

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func (m AppModel) prepareMADE() (AppModel, tea.Cmd) {
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("madeSetup.busy")
		return m, nil
	}
	preparer := m.deps.MADEPreparation
	if preparer == nil {
		m.Status.Error = m.Theme.T("madeSetup.unavailable")
		return m, nil
	}
	m.overlay = "made-setup"
	m.Info = NewTranscript()
	width, height := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(width)
	m.Info.Viewport.SetHeight(height)
	m.Info.SetContent(m.Theme.T("madeSetup.loading"))
	m.Info.Viewport.GotoTop()
	var result application.MADEPreparation
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		result, err = preparer.Prepare(ctx)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.MADEPreparation = &result
			return done
		}
		return msg
	}
}

func madePreparationContent(result application.MADEPreparation, err error, theme Theme) string {
	var b strings.Builder
	if err != nil {
		fmt.Fprintf(&b, "%s\n%s", theme.T("madeSetup.failed"), err)
		return b.String()
	}
	switch result.Status {
	case "ready":
		b.WriteString(theme.Tf("madeSetup.ready", result.WorkIdentity, result.GrantID))
	case "granted":
		b.WriteString(theme.Tf("madeSetup.granted", result.WorkIdentity, result.GrantID))
	case "remote":
		b.WriteString(theme.Tf("madeSetup.remote", result.GrantID))
	case "conflict":
		b.WriteString(theme.Tf("madeSetup.conflict", result.GrantID))
	case "missing":
		b.WriteString(theme.T("madeSetup.missing"))
	default:
		b.WriteString(theme.Tf("madeSetup.unsupported", result.Detail))
	}
	if len(result.Published) > 0 {
		b.WriteString("\n\n" + theme.Tf("madeSetup.published", strings.Join(result.Published, ", ")))
	}
	if result.RestartRequired {
		b.WriteString("\n\n" + theme.T("madeSetup.restart"))
	}
	return b.String()
}
