package terminal

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func (m AppModel) updateEngines() (AppModel, tea.Cmd) {
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("update.busy")
		return m, nil
	}
	updater := m.deps.EngineUpdates
	if updater == nil {
		m.Status.Error = m.Theme.T("update.unavailable")
		return m, nil
	}
	m.overlay = "updates"
	m.Info = NewTranscript()
	width, height := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(width)
	m.Info.Viewport.SetHeight(height)
	m.Info.SetContent(m.Theme.T("update.loading"))
	m.Info.Viewport.GotoTop()
	var results []application.EngineUpdateResult
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		results, err = updater.Update(ctx)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.EngineUpdates = &results
			return done
		}
		return msg
	}
}

func engineUpdateContent(results []application.EngineUpdateResult, err error, theme Theme) string {
	var b strings.Builder
	restart := false
	for _, result := range results {
		fmt.Fprintf(&b, "%s\n", strings.ToUpper(result.Engine))
		switch result.Status {
		case "updated":
			if result.PreviousVersion != "" {
				fmt.Fprintf(&b, "%s → %s · ", result.PreviousVersion, result.Version)
			} else {
				fmt.Fprintf(&b, "%s · ", result.Version)
			}
			b.WriteString(theme.T("update.updated"))
		case "current":
			fmt.Fprintf(&b, "%s · %s", result.Version, theme.T("update.current"))
		case "skipped":
			b.WriteString(theme.T("update.skipped"))
		default:
			fmt.Fprintf(&b, "%s\n%s", theme.T("update.failed"), result.Error)
		}
		b.WriteString("\n\n")
		restart = restart || result.RestartRequired
	}
	if err != nil {
		fmt.Fprintf(&b, "%s\n\n", err)
	}
	if restart {
		b.WriteString(theme.T("update.restart"))
	} else if err == nil {
		b.WriteString(theme.T("update.finished"))
	}
	return b.String()
}
