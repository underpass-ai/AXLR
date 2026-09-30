package terminal

import (
	"context"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type AppModel struct {
	deps                Dependencies
	lifetime            *lifecycle
	Header              Header
	Transcript          Transcript
	Composer            Composer
	Activity            ToolActivity
	Status              StatusBar
	Layout              Layout
	Theme               Theme
	Busy, ActivityTab   bool
	Approval            ApprovalDialog
	SearchBox           SearchBox
	Palette             ActionPalette
	Picker              SessionPicker
	Models              ModelPicker
	Plugins             PluginPanel
	memoryActive        bool
	providerWaiting     bool
	providerWaitStarted time.Time
	waitTickScheduled   bool
	updatingBatch       bool
	operationID         uint64
	operationMessages   int
	streamMessages      int
	streamPending       bool
	draftOperationID    uint64
	Help                HelpOverlay
	Info                Transcript
	overlay             ControlIntent
	draft               string
	submittedPrompt     string
	submittedAt         int
	unsentPrompts       []string
	events              <-chan tea.Msg
	cancel              context.CancelFunc
	zones               *zone.Manager
	prefix              string
}

func assistantInMessages(messages []root.Message, start int) bool {
	for i := len(messages) - 1; i >= start; i-- {
		if messages[i].Role == root.RoleAssistant {
			return true
		}
	}
	return false
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
	m := AppModel{lifetime: &lifecycle{ctx: ctx, cancel: cancel}, deps: deps, Theme: Theme{Monochrome: deps.Monochrome}, Composer: NewComposer(deps.Monochrome), Transcript: NewTranscript(), Plugins: NewPluginPanel(), zones: z, prefix: z.NewPrefix()}
	if deps.Session != nil {
		m.Header.State = deps.Session.Export()
	}
	m.Activity.SetSession(m.Header.State)
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
func (m AppModel) Init() tea.Cmd {
	if m.Theme.Monochrome {
		return nil
	}
	return tea.RequestBackgroundColor
}
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, span := application.StartDiagnosticSpan(m.deps.Context, m.deps.Diagnostics, application.DiagnosticActionUpdate, application.DiagnosticEvent{OperationID: m.operationID})
	defer span.End(application.DiagnosticErrorNone)
	if next, cmd, handled := m.navigation(msg); handled {
		return next, cmd
	}
	switch v := msg.(type) {
	case tea.BackgroundColorMsg:
		m.Theme.Light = !v.IsDark()
		m.Transcript.ApplyTheme(m.Theme)
		return m, nil
	case providerWaitTick:
		if v.OperationID != m.operationID || !m.Busy {
			return m, nil
		}
		m.waitTickScheduled = false
		cmd := m.waitingCommand(nil)
		return m, cmd
	case operationBatch:
		outerBatch := m.updatingBatch
		m.updatingBatch = true
		var cmd tea.Cmd
		for _, item := range v.Messages {
			var next tea.Model
			next, cmd = m.Update(item)
			m = next.(AppModel)
		}
		m.updatingBatch = outerBatch
		cmd = m.waitingCommand(cmd)
		return m, cmd
	case tea.WindowSizeMsg:
		m.record(application.DiagnosticEvent{Stage: application.DiagnosticResize, Width: v.Width, Height: v.Height})
		m.Layout = NewLayout(v.Width, v.Height)
		if m.overlay == "models" {
			m.Models.Input.SetWidth(max(1, v.Width-9))
			m.Models.pageSize = max(1, v.Height-7)
			m.Models.ensureVisible()
		}
		m.Plugins.Resize(v.Width, v.Height-2)
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
		m.Transcript.ApplyTheme(m.Theme)
		return m, nil
	case application.Event:
		m.record(application.DiagnosticEvent{Stage: application.DiagnosticEventConsumed, Chunks: 1, Bytes: len(v.Text)})
		if v.Kind == application.EventSession && v.Snapshot != nil {
			m.providerWaiting = false
			m.Header.State = *v.Snapshot
			m.draft = ""
			m.draftOperationID = 0
			m.refreshTranscript()
		}
		if v.Kind == application.EventToolExecutionStarted {
			m.providerWaiting = false
			m.memoryActive = v.Memory
		}
		if v.Kind == application.EventStreamStart {
			m.providerWaiting = true
			m.providerWaitStarted = time.Now()
			m.memoryActive = false
			m.streamMessages = v.MessageCount
			m.streamPending = true
			if m.draftOperationID == m.operationID {
				m.draft = ""
				m.draftOperationID = 0
			}
		}
		if v.Kind == application.EventTextDelta {
			m.providerWaiting = false
			if m.streamPending || (m.draftOperationID != 0 && m.draftOperationID != m.operationID) {
				m.draft = ""
			}
			m.streamPending = false
			m.draft += string(v.Text)
			m.draftOperationID = m.operationID
			m.refreshTranscript()
		}
		if v.Kind == application.EventState {
			m.Status.State = v.State
		}
		m.Activity.Apply(v)
		if m.events != nil {
			cmd := m.waitingCommand(readOperation(m.events))
			return m, cmd
		}
		cmd := m.waitingCommand(nil)
		return m, cmd
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
		m.providerWaiting = false
		m.waitTickScheduled = false
		m.memoryActive = false
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
		if v.PluginApproval != nil {
			for i := range m.Plugins.Items {
				if m.Plugins.Items[i].Profile.ID == v.PluginApproval.ID {
					m.Plugins.Items[i].Profile.Approval = v.PluginApproval.Approval
				}
			}
		}
		if v.Plugins != nil {
			m.Plugins.Loading = false
			if v.Err != nil {
				m.Plugins.Error = v.Err.Error()
				if v.PluginApproval != nil {
					m.Plugins.Error = "Approval saved; inventory refresh failed: " + v.Err.Error()
				}
			} else {
				m.Plugins.SetItems(*v.Plugins)
			}
			m.Plugins.Resize(m.Layout.Width, m.Layout.Height-2)
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
			m.Activity.SetSession(m.Header.State)
			m.unsentPrompts = nil
			m.draft = ""
			m.draftOperationID = 0
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
		state := v.Session.Export()
		if assistantInMessages(state.Messages, m.streamMessages) || (m.draftOperationID == v.ID && state.Draft == root.Text(m.draft)) {
			m.draft = ""
			m.draftOperationID = 0
		}
		if v.Err != nil {
			m.Status.Error = v.Err.Error()
			if v.PluginApproval != nil {
				m.Status.Error = "Approval saved; inventory refresh failed: " + v.Err.Error()
			}
		} else if m.draft == "" {
			m.Status.Error = ""
		}
		if v.ModelSelection && v.Err == nil && v.PreferenceErr != nil {
			m.Status.Error = "Model selected, but its default could not be saved"
		}
		m.refreshTranscript()
		m.syncApproval()
		return m, nil
	case ControlIntent:
		switch v {
		case "send":
			command := strings.TrimSpace(m.Composer.Input.Value())
			if command == "/mcp" || command == "/plugin" {
				intent := ControlIntent("mcp")
				if command == "/plugin" {
					intent = "plugins"
				}
				next, cmd, _ := m.navigation(intent)
				changed := next.(AppModel)
				if cmd != nil {
					changed.Composer.Input.Reset()
				}
				return changed, cmd
			}
			if command == "/model" {
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
			if m.Header.State.Status != domain.StatusIdle && m.Header.State.Status != domain.StatusComplete && m.Header.State.Status != domain.StatusInterrupted {
				m.Status.Error = "Finish or continue the current turn before sending another prompt"
				return m, nil
			}
			if m.deps.Session == nil || len(m.deps.Session.Pending()) != 0 {
				m.Status.Error = "Resolve pending tool calls before sending another prompt"
				return m, nil
			}
			prompt := root.Text(m.Composer.Input.Value())
			m.record(application.DiagnosticEvent{Stage: application.DiagnosticInputSubmitted, OperationID: m.operationID + 1, Bytes: len(prompt), Messages: len(m.Header.State.Messages) + 1})
			m.submittedPrompt = string(prompt)
			m.submittedAt = len(m.Header.State.Messages)
			m.Composer.Input.Reset()
			m.Status.Error = ""
			m.draft = ""
			m.draftOperationID = 0
			if m.Header.State.Status == domain.StatusInterrupted && m.Header.State.Draft != "" {
				m.Header.State.ArchivedDrafts = append(m.Header.State.ArchivedDrafts, domain.ArchivedDraft{AfterMessage: len(m.Header.State.Messages), Content: m.Header.State.Draft})
				m.Header.State.Draft = ""
			}
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
	_, span := application.StartDiagnosticSpan(m.deps.Context, m.deps.Diagnostics, application.DiagnosticActionRender, application.DiagnosticEvent{OperationID: m.operationID, Width: max(0, m.Layout.Width), Height: max(0, m.Layout.Height)})
	defer span.End(application.DiagnosticErrorNone)
	var started time.Time
	if m.deps.Diagnostics != nil {
		started = time.Now()
	}
	var content string
	if m.Layout.TooSmall || m.Layout.Width == 0 {
		content = ansi.Truncate("Resize terminal to at least 50 × 15", max(1, m.Layout.Width), "")
	} else {
		body := m.Transcript.View()

		if m.ActivityTab {
			body = m.Activity.View(m.Layout.Width, m.Layout.BodyHeight)
		}
		tabs := m.Activity.Tabs(m.zones, m.prefix)
		if m.memoryActive {
			tabs = m.Theme.MemoryRow().Render("Memory · KMP is running")
		}
		body = lipgloss.JoinVertical(lipgloss.Left, tabs, body)
		controls := m.Composer.Controls(m.zones, m.prefix)
		content = lipgloss.JoinVertical(lipgloss.Left, m.Header.View(m.Layout.Width, m.Theme), body, m.Composer.View(), controls, m.statusView())
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
			view.Cursor.Y += 2 + m.Layout.BodyHeight
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
	if m.deps.Diagnostics != nil {
		m.record(application.DiagnosticEvent{Stage: application.DiagnosticRender, Width: lipgloss.Width(content), Height: lipgloss.Height(content), ElapsedMilliseconds: time.Since(started).Milliseconds()})
	}
	return view
}

// Failed submissions remain display state, never valid model history.
func (m *AppModel) refreshTranscript() {
	m.Activity.SetSession(m.Header.State)
	m.Transcript.SetSession(m.Header.State, m.draft, m.Theme)
	if len(m.unsentPrompts) > 0 {
		m.Transcript.AppendUnsent(m.unsentPrompts)
	}
}
