package terminal

import (
	"context"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
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
	tokens              int
	Status              StatusBar
	Layout              Layout
	Theme               Theme
	UIPreferences       domain.UIPreferences
	ThemePicker         ThemePicker
	Busy                bool
	Approval            ApprovalDialog
	SearchBox           SearchBox
	Palette             ActionPalette
	Picker              SessionPicker
	Models              ModelPicker
	Plugins             PluginPanel
	InstalledPlugins    InstalledPlugins
	Changes             ChangeViewer
	memoryActive        bool
	providerWaiting     bool
	providerWaitStarted time.Time
	waitTickScheduled   bool
	activitySpinner     spinner.Model
	spinnerScheduled    bool
	toolExecuting       bool
	toolStarted         time.Time
	toolName            string
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
	steerPrompt         string
	promptHistory       []string
	historyIndex        int
	historyDraft        string
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
	if deps.Locale != Spanish {
		deps.Locale = English
	}
	deps.Monochrome = deps.Monochrome || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	if deps.UIPreferences.Theme == "" {
		deps.UIPreferences = domain.DefaultUIPreferences()
	}
	parent := deps.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	z := zone.New()
	m := AppModel{lifetime: &lifecycle{ctx: ctx, cancel: cancel}, deps: deps, UIPreferences: deps.UIPreferences, Theme: Theme{ID: deps.UIPreferences.Theme, Icons: deps.UIPreferences.Icons, Locale: deps.Locale, Monochrome: deps.Monochrome}, Composer: NewComposer(deps.Monochrome, deps.Locale), Transcript: NewTranscript(), Plugins: NewPluginPanel(), InstalledPlugins: NewInstalledPlugins(), activitySpinner: spinner.New(spinner.WithSpinner(spinner.Spinner{Frames: []string{"◐", "◓", "◑", "◒"}, FPS: 125 * time.Millisecond})), zones: z, prefix: z.NewPrefix()}
	m.Composer.Theme = m.Theme
	m.Changes = NewChangeViewer()
	m.Changes.Theme = m.Theme
	m.Composer.Input.Placeholder = m.Theme.T("composer.placeholder")
	m.Transcript.Gutter = 2
	m.Models.Theme = m.Theme
	m.Models.Input.Prompt = m.Theme.T("common.searchPrompt")
	m.Plugins.Theme = m.Theme
	m.InstalledPlugins.Theme = m.Theme
	m.Plugins.Search.Placeholder = m.Theme.T("plugins.searchPlaceholder")
	if deps.Session != nil {
		m.Header.State = deps.Session.Export()
	}
	m.resetPromptHistory()
	if m.Header.State.ID == "" {
		m.Header.State.Workspace = deps.Workspace
	}
	m.Status.State = m.Header.State.Status
	m.Status.Locale = m.Theme.Locale
	if deps.ApprovalSettings != nil {
		m.Status.Autonomous = deps.ApprovalSettings.Autonomous()
	}
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
		m.applyUIPreferences(m.UIPreferences)
		return m, nil
	case providerWaitTick:
		if v.OperationID != m.operationID || !m.Busy {
			return m, nil
		}
		m.waitTickScheduled = false
		cmd := m.waitingCommand(nil)
		return m, cmd
	case providerAnimationTick:
		if v.OperationID != m.operationID || !m.Busy {
			return m, nil
		}
		m.spinnerScheduled = false
		if !m.providerWaiting && !m.toolExecuting {
			return m, nil
		}
		m.activitySpinner, _ = m.activitySpinner.Update(v.Tick)
		return m, m.waitingCommand(nil)
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
		m.Changes.Resize(v.Width, v.Height-1)
		m.InstalledPlugins.Resize(v.Width, v.Height-2)
		offset, bottom := m.Transcript.Viewport.YOffset(), m.Transcript.Viewport.AtBottom()
		m.Transcript.SetWidth(max(1, m.Layout.TranscriptWidth-2*m.Transcript.Gutter))
		m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
		if bottom {
			// Rewrapping changes the line count; keep the latest turn in view.
			m.Transcript.Viewport.GotoBottom()
		} else {
			m.Transcript.Viewport.SetYOffset(offset)
		}
		m.Composer.Input.SetWidth(max(1, v.Width))
		m.resizeComposer()
		m.SearchBox.Input.SetWidth(max(1, v.Width-18))
		m.SearchBox.Input.SetCursor(m.SearchBox.Input.Position())
		m.sizeApproval()
		infoWidth, infoHeight := OverlayBodySize(v.Width, v.Height-1)
		m.Info.SetWidth(infoWidth)
		m.Info.Viewport.SetHeight(infoHeight)
		m.Transcript.ApplyTheme(m.Theme)
		return m, nil
	case application.Event:
		m.record(application.DiagnosticEvent{Stage: application.DiagnosticEventConsumed, Chunks: 1, Bytes: len(v.Text)})
		if v.Kind == application.EventSession && v.Snapshot != nil {
			m.providerWaiting = false
			m.toolExecuting = false
			m.Header.State = *v.Snapshot
			m.draft = ""
			m.draftOperationID = 0
			m.refreshTranscript()
		}
		if v.Kind == application.EventToolExecutionStarted {
			m.providerWaiting = false
			m.toolExecuting = true
			m.toolStarted = time.Now()
			m.toolName = singleLine(string(v.Tool.Call.Name))
			m.memoryActive = v.Memory
		}
		if v.Kind == application.EventStreamStart {
			if m.steerPrompt != "" && m.cancel != nil {
				m.cancel()
			}
			m.toolExecuting = false
			m.Status.Phase = domain.ProviderWaiting
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
			m.toolExecuting = false
			if m.streamPending || (m.draftOperationID != 0 && m.draftOperationID != m.operationID) {
				m.draft = ""
			}
			m.streamPending = false
			m.draft += string(v.Text)
			m.draftOperationID = m.operationID
			m.refreshTranscript()
		}
		if v.Kind == application.EventProviderActivity && m.Busy && m.streamPending {
			m.Status.Phase = v.ProviderPhase
			m.providerWaiting = v.ProviderPhase != domain.ProviderContent
		}
		if v.Kind == application.EventState {
			m.Status.State = v.State
		}
		if v.Usage != nil {
			m.tokens = v.Usage.TotalTokens
		}
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
		m.spinnerScheduled = false
		m.toolExecuting = false
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
					m.Plugins.Error = m.Theme.T("error.approvalRefresh") + v.Err.Error()
				}
			} else {
				m.Plugins.SetItems(*v.Plugins)
			}
			m.Plugins.Resize(m.Layout.Width, m.Layout.Height-2)
		}
		if v.InstalledPlugins != nil {
			m.InstalledPlugins.Loading = false
			if v.Err != nil {
				m.InstalledPlugins.Error = v.Err.Error()
			} else {
				m.InstalledPlugins.SetItems(*v.InstalledPlugins)
			}
			m.InstalledPlugins.Resize(m.Layout.Width, m.Layout.Height-2)
		}
		if v.EngineUpdates != nil {
			m.Info.SetContent(engineUpdateContent(*v.EngineUpdates, v.Err, m.Theme))
			m.Info.Viewport.GotoTop()
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
			m.tokens = 0
			m.unsentPrompts = nil
			m.resetPromptHistory()
			m.draft = ""
			m.draftOperationID = 0
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
		if m.deps.ApprovalSettings != nil {
			m.Status.Autonomous = m.deps.ApprovalSettings.Autonomous()
		}
		state := v.Session.Export()
		if assistantInMessages(state.Messages, m.streamMessages) || (m.draftOperationID == v.ID && state.Draft == root.Text(m.draft)) {
			m.draft = ""
			m.draftOperationID = 0
		}
		if v.Err != nil {
			m.Status.Error = v.Err.Error()
			if v.PluginApproval != nil {
				m.Status.Error = m.Theme.T("error.approvalRefresh") + v.Err.Error()
			}
		} else if m.draft == "" {
			m.Status.Error = ""
		}
		if v.ModelSelection && v.Err == nil && v.PreferenceErr != nil {
			m.Status.Error = m.Theme.T("error.modelDefault")
		}
		m.refreshTranscript()
		m.syncApproval()
		if m.steerPrompt != "" {
			prompt := root.Text(m.steerPrompt)
			m.steerPrompt = ""
			m.submittedPrompt = string(prompt)
			m.submittedAt = len(m.Header.State.Messages)
			start := m.deps.Start
			store := m.deps.Store
			cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
				if len(s.Pending()) != 0 {
					next := *s
					if err := next.CancelPending(); err != nil {
						return err
					}
					if err := store.Save(ctx, next); err != nil {
						return err
					}
					*s = next
				}
				return start.Execute(ctx, s, prompt, emit)
			})
			return m, cmd
		}
		return m, nil
	case ControlIntent:
		switch v {
		case "send":
			command := strings.TrimSpace(m.Composer.Input.Value())
			if command == "/update" {
				changed, cmd := m.updateEngines()
				if cmd != nil {
					changed.Composer.Input.Reset()
				}
				return changed, cmd
			}
			if command == "/approvals" {
				m.Composer.Input.Reset()
				return m.Update(ControlIntent("approvals"))
			}
			if command == "/changes" || command == "/diff" {
				m.Composer.Input.Reset()
				return m.Update(ControlIntent("changes"))
			}
			if command == "/autonomy on" || command == "/autonomy off" || command == "/autonomy" {
				settings := m.deps.ApprovalSettings
				if settings == nil {
					m.Status.Error = m.Theme.T("error.approvalSettings")
					return m, nil
				}
				if command != "/autonomy" {
					if err := settings.SetAutonomous(m.lifetime.ctx, command == "/autonomy on"); err != nil {
						m.Status.Error = err.Error()
						return m, nil
					}
				}
				m.Status.Autonomous = settings.Autonomous()
				m.Status.Error = ""
				m.Composer.Input.Reset()
				return m, nil
			}
			if command == "/theme" {
				m.Composer.Input.Reset()
				next, cmd, _ := m.navigation(ControlIntent("theme"))
				return next, cmd
			}
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
			if m.Busy && command != "" && (m.submittedPrompt != "" || m.Header.State.Status == domain.StatusStreaming || m.Header.State.Status == domain.StatusApproval || m.toolExecuting || m.providerWaiting) {
				message := m.Composer.Input.Value()
				m.rememberPrompt(message)
				if m.steerPrompt == "" {
					m.steerPrompt = message
				} else {
					m.steerPrompt += "\n\n" + message
				}
				m.Composer.Input.Reset()
				if !m.toolExecuting && m.Header.State.Status != domain.StatusApproval && (m.providerWaiting || m.streamPending || m.draftOperationID == m.operationID) && m.cancel != nil {
					m.cancel()
				}
				return m, nil
			}
			if m.Busy || strings.TrimSpace(m.Composer.Input.Value()) == "" {
				return m, nil
			}
			if m.Header.State.ID == "" {
				m.Status.Error = m.Theme.T("error.noModel")
				return m, nil
			}
			if m.Header.State.Status != domain.StatusIdle && m.Header.State.Status != domain.StatusComplete && m.Header.State.Status != domain.StatusInterrupted {
				m.Status.Error = m.Theme.T("error.turnActive")
				return m, nil
			}
			if m.deps.Session == nil || len(m.deps.Session.Pending()) != 0 {
				m.Status.Error = m.Theme.T("error.promptPending")
				return m, nil
			}
			prompt := root.Text(m.Composer.Input.Value())
			m.rememberPrompt(string(prompt))
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
		}
	case tea.KeyPressMsg:
		switch v.String() {
		case "up":
			if m.overlay == "" && (m.historyIndex < len(m.promptHistory) || m.Composer.Input.Line() == 0) && m.previousPrompt() {
				return m, nil
			}
		case "down":
			if m.overlay == "" && m.historyIndex < len(m.promptHistory) && m.nextPrompt() {
				return m, nil
			}
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
		case "pgup":
			m.Transcript.Viewport.PageUp()
			return m, nil
		case "pgdown":
			m.Transcript.Viewport.PageDown()
			return m, nil
		}
	case tea.MouseClickMsg:
		if v.Button == tea.MouseLeft {
			for _, id := range []string{"send", "cancel"} {
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
	m.resizeComposer()
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
		content = ansi.Truncate(m.Theme.T("app.resize"), max(1, m.Layout.Width), "")
	} else {
		composer, footer := m.Composer.View(m.Layout.Width), m.footerView()
		if m.inlineApproval() {
			composer = m.approvalCard()
		} else if m.overlay == "search" {
			composer, footer = m.searchArea(), m.searchFooter()
		}
		content = lipgloss.JoinVertical(lipgloss.Left, m.Header.View(m.Layout.Width, m.Theme), m.mainTranscript(), composer, footer)
	}
	if !m.Layout.TooSmall && m.Layout.Width > 0 {
		content = m.overlayView(content)
	}
	content = m.zones.Scan(content)
	if m.Theme.Monochrome {
		content = ansi.Strip(content)
	}
	view := tea.NewView(content)
	if !m.Theme.Monochrome && m.Theme.ID != "" && m.Theme.ID != domain.ThemeAuto {
		p := m.Theme.palette()
		view.BackgroundColor = lipgloss.Color(p.Background)
		view.ForegroundColor = lipgloss.Color(p.Text)
	}
	if !m.Layout.TooSmall && m.Layout.Width > 0 && !m.approvalFocus() && m.overlay == "" {
		view.Cursor = m.Composer.Input.Cursor()
		if view.Cursor != nil {
			// Header and the rule above the composer.
			view.Cursor.Y += 2 + m.Layout.BodyHeight
		}
	}
	if !m.Layout.TooSmall && m.Layout.Width > 0 && m.overlay == "models" && !m.approvalFocus() {
		view.Cursor = m.Models.Input.Cursor()
		if view.Cursor != nil {
			view.Cursor.Y++
		}
	}
	if !m.Layout.TooSmall && m.overlay == "palette" && m.Palette.List.SettingFilter() && !m.approvalFocus() {
		view.Cursor = m.Palette.List.FilterInput.Cursor()
		if view.Cursor != nil {
			view.Cursor.X += 2
			view.Cursor.Y += 4
		}
	}
	if !m.Layout.TooSmall && m.overlay == "mcp" && (m.Plugins.searching || m.Plugins.installing) {
		if m.Plugins.installing {
			view.Cursor = m.Plugins.InstallInput.Cursor()
		} else {
			view.Cursor = m.Plugins.Search.Cursor()
		}
		if view.Cursor != nil {
			if m.Plugins.installing {
				view.Cursor.Y += 4
			} else {
				view.Cursor.Y++
			}
		}
	}
	if !m.Layout.TooSmall && m.overlay == "plugins" && (m.InstalledPlugins.searching || m.InstalledPlugins.addingMarketplace) {
		if m.InstalledPlugins.addingMarketplace {
			view.Cursor = m.InstalledPlugins.MarketplaceInput.Cursor()
		} else {
			view.Cursor = m.InstalledPlugins.Search.Cursor()
		}
		if view.Cursor != nil {
			if m.InstalledPlugins.addingMarketplace {
				view.Cursor.Y += 4
			} else {
				view.Cursor.Y++
			}
		}
	}
	if !m.Layout.TooSmall && m.overlay == "search" && !m.approvalFocus() {
		view.Cursor = m.SearchBox.Input.Cursor()
		if view.Cursor != nil {
			// Header, conversation and the rule above the search row.
			view.Cursor.Y += 2 + m.Layout.BodyHeight
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
	m.Changes.SetSession(m.Header.State)
	m.Transcript.SetSession(m.Header.State, m.draft, m.Theme)
	if len(m.unsentPrompts) > 0 {
		m.Transcript.AppendUnsent(m.unsentPrompts)
	}
}
