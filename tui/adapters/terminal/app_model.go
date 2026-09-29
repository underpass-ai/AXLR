package terminal

import (
	"context"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type AppModel struct {
	deps              Dependencies
	Header            Header
	Transcript        Transcript
	Composer          Composer
	Activity          ToolActivity
	Status            StatusBar
	Layout            Layout
	Theme             Theme
	Busy, ActivityTab bool
	draft             string
	events            <-chan tea.Msg
	cancel            context.CancelFunc
	zones             *zone.Manager
	prefix            string
}

var _ tea.Model = AppModel{}

func New(deps Dependencies) AppModel {
	deps.Monochrome = deps.Monochrome || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	z := zone.New()
	m := AppModel{deps: deps, Theme: Theme{deps.Monochrome}, Composer: NewComposer(deps.Monochrome), Transcript: NewTranscript(), zones: z, prefix: z.NewPrefix()}
	if deps.Session != nil {
		m.Header.State = deps.Session.Export()
	}
	m.Status.State = m.Header.State.Status
	m.Transcript.SetSession(m.Header.State, "")
	return m
}
func (m AppModel) Init() tea.Cmd { return nil }
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.Layout = NewLayout(v.Width, v.Height)
		offset := m.Transcript.Viewport.YOffset()
		m.Transcript.Viewport.SetWidth(m.Layout.TranscriptWidth)
		m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
		m.Transcript.Viewport.SetYOffset(offset)
		m.Composer.Input.SetWidth(max(1, v.Width))
		return m, nil
	case application.Event:
		if v.Kind == application.EventTextDelta {
			m.draft += string(v.Text)
			m.Transcript.SetSession(m.Header.State, m.draft)
		}
		if v.Kind == application.EventState {
			m.Status.State = v.State
		}
		m.Activity.Apply(v)
		if m.events != nil {
			return m, readOperation(m.events)
		}
		return m, nil
	case operationComplete:
		if m.cancel != nil {
			m.cancel()
		}
		m.cancel = nil
		m.events = nil
		m.Busy = false
		if m.deps.Session != nil {
			*m.deps.Session = v.Session
		}
		m.Header.State = v.Session.Export()
		m.Status.State = v.Session.Status()
		m.draft = ""
		if v.Err != nil {
			m.Status.Error = v.Err.Error()
		}
		m.Transcript.SetSession(m.Header.State, "")
		return m, nil
	case ControlIntent:
		switch v {
		case "send":
			if m.Busy || strings.TrimSpace(m.Composer.Input.Value()) == "" {
				return m, nil
			}
			prompt := root.Text(m.Composer.Input.Value())
			m.Composer.Input.Reset()
			m.Status.Error = ""
			m.draft = ""
			m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: prompt})
			m.Transcript.SetSession(m.Header.State, "")
			cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
				return m.deps.Start.Execute(ctx, s, prompt, emit)
			})
			return m, cmd
		case "cancel":
			if m.cancel != nil {
				m.cancel()
			}
			return m, nil
		case "activity":
			m.ActivityTab = true
			return m, nil
		case "transcript":
			m.ActivityTab = false
			return m, nil
		}
	case tea.KeyPressMsg:
		switch v.String() {
		case "ctrl+s":
			return m.Update(m.Composer.Intent(v))
		case "esc":
			return m.Update(ControlIntent("cancel"))
		case "ctrl+c":
			if m.Busy {
				return m.Update(ControlIntent("cancel"))
			}
			m.zones.Close()
			return m, tea.Quit
		case "tab":
			m.ActivityTab = !m.ActivityTab
			return m, nil
		case "pgup":
			m.Transcript.Viewport.PageUp()
			return m, nil
		case "pgdown":
			m.Transcript.Viewport.PageDown()
			return m, nil
		}
	case tea.MouseClickMsg:
		if v.Button == tea.MouseLeft {
			for _, id := range []string{"send", "cancel", "activity", "transcript"} {
				if m.zones.Get(m.prefix + id).InBounds(v) {
					return m.Update(ControlIntent(id))
				}
			}
		}
		return m, nil
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.Transcript.Viewport, cmd = m.Transcript.Viewport.Update(v)
		return m, cmd
	}
	var cmd tea.Cmd
	m.Composer, cmd = m.Composer.Update(msg)
	return m, cmd
}
func (m AppModel) View() tea.View {
	var content string
	if m.Layout.TooSmall || m.Layout.Width == 0 {
		content = ansi.Truncate("Resize terminal to at least 50 × 15", max(1, m.Layout.Width), "")
	} else {
		body := m.Transcript.View()
		if m.Layout.SidePanel {
			body = lipgloss.JoinHorizontal(lipgloss.Top, body, " ", m.Activity.View(28, m.Layout.BodyHeight))
		} else {
			if m.ActivityTab {
				body = m.Activity.View(m.Layout.Width, m.Layout.BodyHeight)
			}
			body = lipgloss.JoinVertical(lipgloss.Left, m.Activity.Tabs(m.zones, m.prefix), body)
		}
		controls := m.Composer.Controls(m.zones, m.prefix)
		content = lipgloss.JoinVertical(lipgloss.Left, m.Header.View(m.Layout.Width, m.Theme), body, m.Composer.View(), controls, m.Status.View(m.Layout.Width))
	}
	content = m.zones.Scan(content)
	if m.Theme.Monochrome {
		content = ansi.Strip(content)
	}
	view := tea.NewView(content)
	if !m.Layout.TooSmall && m.Layout.Width > 0 {
		view.Cursor = m.Composer.Input.Cursor()
		if view.Cursor != nil {
			view.Cursor.Y += 1 + m.Layout.BodyHeight
			if !m.Layout.SidePanel {
				view.Cursor.Y++
			}
		}
	}
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
