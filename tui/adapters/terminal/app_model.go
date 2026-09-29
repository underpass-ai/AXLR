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
	lifetime          *lifecycle
	Header            Header
	Transcript        Transcript
	Composer          Composer
	Activity          ToolActivity
	Status            StatusBar
	Layout            Layout
	Theme             Theme
	Busy, ActivityTab bool
	Approval          ApprovalDialog
	SearchBox         SearchBox
	Palette           ActionPalette
	Picker            SessionPicker
	Models            ModelPicker
	operationID       uint64
	Help              HelpOverlay
	Info              Transcript
	overlay           ControlIntent
	draft             string
	submittedPrompt   string
	submittedAt       int
	unsentPrompts     []string
	events            <-chan tea.Msg
	cancel            context.CancelFunc
	zones             *zone.Manager
	prefix            string
}

var _ tea.Model = AppModel{}

func New(deps Dependencies) AppModel {
	deps.Monochrome = deps.Monochrome || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	parent := deps.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	z := zone.New()
	m := AppModel{lifetime: &lifecycle{ctx: ctx, cancel: cancel}, deps: deps, Theme: Theme{deps.Monochrome}, Composer: NewComposer(deps.Monochrome), Transcript: NewTranscript(), zones: z, prefix: z.NewPrefix()}
	if deps.Session != nil {
		m.Header.State = deps.Session.Export()
	}
	for _, record := range m.Header.State.Activity {
		m.Activity.Apply(application.Event{Kind: application.EventToolActivity, Tool: record})
	}
	if m.Header.State.ID == "" {
		m.Header.State.Workspace = deps.Workspace
	}
	m.Status.State = m.Header.State.Status
	m.refreshTranscript()
	m.syncApproval()
	return m
}
func (m AppModel) Init() tea.Cmd { return nil }
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.navigation(msg); handled {
		return next, cmd
	}
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.Layout = NewLayout(v.Width, v.Height)
		if m.overlay == "models" {
			m.Models.Input.SetWidth(max(1, v.Width-9))
			m.Models.pageSize = max(1, v.Height-7)
			m.Models.ensureVisible()
		}
		offset := m.Transcript.Viewport.YOffset()
		m.Transcript.Viewport.SetWidth(m.Layout.TranscriptWidth)
		m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
		m.Transcript.Viewport.SetYOffset(offset)
		m.Composer.Input.SetWidth(max(1, v.Width))
		m.SearchBox.Input.SetWidth(max(1, v.Width-18))
		m.SearchBox.Input.SetCursor(m.SearchBox.Input.Position())
		m.sizeApproval()
		m.Info.Viewport.SetWidth(max(1, v.Width))
		m.Info.Viewport.SetHeight(max(1, v.Height-4))
		return m, nil
	case application.Event:
		if v.Kind == application.EventTextDelta {
			m.draft += string(v.Text)
			m.refreshTranscript()
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
		if v.ID != m.operationID || !m.Busy {
			return m, nil
		}
		if m.cancel != nil {
			m.cancel()
		}
		m.cancel = nil
		m.events = nil
		m.Busy = false
		if v.Session.Export().ID != "" {
			if m.deps.Session == nil {
				m.deps.Session = new(domain.Session)
			}
			*m.deps.Session = v.Session
		}
		oldID := m.Header.State.ID
		if v.Session.Export().ID != "" {
			m.Header.State = v.Session.Export()
		}
		if v.Models != nil {
			if v.Err != nil {
				m.Models.SetError(v.Err)
			} else {
				m.Models.SetModels(*v.Models)
			}
		}
		if v.ModelSelection {
			if v.Err != nil {
				m.Models.SetError(v.Err)
			} else {
				m.overlay = ""
			}
		}
		if v.Sessions != nil {
			m.Picker = SessionPicker{Items: *v.Sessions}
			m.overlay = "sessions"
		}
		if oldID != m.Header.State.ID {
			m.overlay = ""
			m.SearchBox = SearchBox{}
			m.Activity = ToolActivity{}
			m.unsentPrompts = nil
			for _, record := range m.Header.State.Activity {
				m.Activity.Apply(application.Event{Kind: application.EventToolActivity, Tool: record})
			}
		}
		if m.submittedPrompt != "" && v.Err != nil {
			messages := m.Header.State.Messages
			accepted := len(messages) > m.submittedAt && messages[m.submittedAt].Role == root.RoleUser && string(messages[m.submittedAt].Content) == m.submittedPrompt
			if !accepted {
				if m.Composer.Input.Value() == "" {
					m.Composer.Input.SetValue(m.submittedPrompt)
				} else {
					m.unsentPrompts = append(m.unsentPrompts, m.submittedPrompt)
				}
			}
		}
		m.submittedPrompt = ""
		m.Status.State = v.Session.Status()
		m.draft = ""
		if v.Err != nil {
			m.Status.Error = v.Err.Error()
		}
		m.refreshTranscript()
		m.syncApproval()
		return m, nil
	case ControlIntent:
		switch v {
		case "send":
			if m.Composer.Input.Value() == "/model" {
				next, cmd, _ := m.navigation(ControlIntent("models"))
				changed := next.(AppModel)
				if cmd != nil {
					changed.Composer.Input.Reset()
				}
				return changed, cmd
			}
			if m.Busy || strings.TrimSpace(m.Composer.Input.Value()) == "" {
				return m, nil
			}
			if m.Header.State.ID == "" {
				m.Status.Error = "Select a model with /model before sending"
				return m, nil
			}
			prompt := root.Text(m.Composer.Input.Value())
			m.submittedPrompt = string(prompt)
			m.submittedAt = len(m.Header.State.Messages)
			m.Composer.Input.Reset()
			m.Status.Error = ""
			m.draft = ""
			m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: prompt})
			m.refreshTranscript()
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
		case "enter":
			if m.Composer.Input.Value() == "/model" {
				return m.Update(ControlIntent("send"))
			}
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
	if !m.Layout.TooSmall && m.Layout.Width > 0 {
		content = m.overlayView(content)
	}
	content = m.zones.Scan(content)
	if m.Theme.Monochrome {
		content = ansi.Strip(content)
	}
	view := tea.NewView(content)
	if !m.Layout.TooSmall && m.Layout.Width > 0 && !m.approvalFocus() && m.overlay == "" {
		view.Cursor = m.Composer.Input.Cursor()
		if view.Cursor != nil {
			view.Cursor.Y += 1 + m.Layout.BodyHeight
			if !m.Layout.SidePanel {
				view.Cursor.Y++
			}
		}
	}
	if !m.Layout.TooSmall && m.Layout.Width > 0 && m.overlay == "models" && !m.approvalFocus() {
		view.Cursor = m.Models.Input.Cursor()
		if view.Cursor != nil {
			view.Cursor.Y++
		}
	}
	if !m.Layout.TooSmall && m.overlay == "search" && !m.approvalFocus() {
		view.Cursor = m.SearchBox.Input.Cursor()
		if view.Cursor != nil {
			view.Cursor.Y += m.Layout.BodyHeight
		}
	}
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// Failed submissions remain display state, never valid model history.
func (m *AppModel) refreshTranscript() {
	m.Transcript.SetSession(m.Header.State, m.draft)
	if len(m.unsentPrompts) > 0 {
		content := m.Transcript.Viewport.GetContent()
		for _, prompt := range m.unsentPrompts {
			content += "\nNot sent: " + prompt
		}
		m.Transcript.SetContent(content)
	}
}
