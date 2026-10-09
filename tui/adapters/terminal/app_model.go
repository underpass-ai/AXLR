package terminal

import (
	"context"
	"errors"
	"fmt"
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
	deps       Dependencies
	lifetime   *lifecycle
	Header     Header
	Transcript Transcript
	Composer   Composer
	tokens     int
	// cached is the share, in percent, of the last request's prompt that the
	// provider read from its prompt cache; zero without a cache.
	cached           int
	Status           StatusBar
	Layout           Layout
	Theme            Theme
	UIPreferences    domain.UIPreferences
	ThemePicker      ThemePicker
	Busy             bool
	Approval         ApprovalDialog
	SearchBox        SearchBox
	Palette          ActionPalette
	Picker           SessionPicker
	Models           ModelPicker
	Plugins          PluginPanel
	InstalledPlugins InstalledPlugins
	Changes          ChangeViewer
	memoryActive     bool
	slashSelected    int
	// lastClick, lastClickAt and clickCount tell a double or triple click
	// on the transcript from separate clicks.
	lastClick           Selection
	lastClickAt         time.Time
	clickCount          int
	providerWaiting     bool
	providerWaitStarted time.Time
	waitTickScheduled   bool
	activitySpinner     spinner.Model
	spinnerScheduled    bool
	toolExecuting       bool
	toolStarted         time.Time
	toolName            string
	// toolCallName and toolCallBytes describe the tool call being streamed.
	toolCallName      string
	toolCallBytes     int
	updatingBatch     bool
	operationID       uint64
	operationMessages int
	streamMessages    int
	streamPending     bool
	draftOperationID  uint64
	Help              HelpOverlay
	Info              Transcript
	IncidentCard      IncidentCard
	RepairPanel       RepairPanel
	PlanPanel         PlanPanel
	overlay           ControlIntent
	draft             string
	submittedPrompt   string
	submittedAt       int
	unsentPrompts     []string
	// steer holds what the person wrote while the operation ran; the running
	// turn takes it after a tool step, or it starts the next turn.
	steer *steerQueue
	// steerCancelled marks the operation the console stopped for a queued
	// message, so its cancellation is not reported as an error.
	steerCancelled uint64
	promptHistory  []string
	historyIndex   int
	historyDraft   string
	events         <-chan tea.Msg
	cancel         context.CancelFunc
	zones          *zone.Manager
	prefix         string

	// closedCancelled marks the operation stopped because the person closed
	// its panel (the model picker while it loads); likewise not an error.
	closedCancelled uint64
}

// turnRunning reports whether the busy operation is a turn, which takes a
// queued message; other operations (updates, lists, preparation) do not.
func (m AppModel) turnRunning() bool {
	return m.submittedPrompt != "" || m.Header.State.Status == domain.StatusStreaming || m.Header.State.Status == domain.StatusApproval || m.toolExecuting || m.providerWaiting
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
	m := AppModel{lifetime: &lifecycle{ctx: ctx, cancel: cancel}, deps: deps, UIPreferences: deps.UIPreferences, Theme: Theme{ID: deps.UIPreferences.Theme, Icons: deps.UIPreferences.Icons, Locale: deps.Locale, Monochrome: deps.Monochrome}, Composer: NewComposer(deps.Monochrome, deps.Locale), Transcript: NewTranscript(), Plugins: NewPluginPanel(), InstalledPlugins: NewInstalledPlugins(), activitySpinner: spinner.New(spinner.WithSpinner(spinner.Spinner{Frames: []string{"◐", "◓", "◑", "◒"}, FPS: 125 * time.Millisecond})), zones: z, prefix: z.NewPrefix(), steer: &steerQueue{}}
	m.Composer.ApplyTheme(m.Theme)
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
	if deps.InitialDraft != "" {
		m.Composer.Input.SetValue(deps.InitialDraft)
	}
	m.refreshTranscript()
	m.syncApproval()
	return m.loadRepairs().loadPlans()
}
func (m AppModel) Init() tea.Cmd {
	repairs := m.subscribeRepairs()
	if plans := m.subscribePlans(); plans != nil {
		repairs = tea.Batch(repairs, plans)
	}
	if m.Theme.Monochrome {
		return repairs
	}
	if repairs == nil {
		return tea.RequestBackgroundColor
	}
	return tea.Batch(tea.RequestBackgroundColor, repairs)
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
	case planEventMsg:
		m = m.loadPlans()
		m = m.refreshPlanPanel()
		return m, m.subscribePlans()
	case repairEventMsg:
		// A repair session changed: reread the registry, redraw the panel,
		// open it when a decision waits and nothing else is on screen, and
		// keep listening.
		m = m.loadRepairs()
		m = m.refreshRepairPanel()
		m = m.autoOpenRepairPanel()
		return m, m.subscribeRepairs()
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
		m.clearSelection()
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
		if m.overlay == "incident" || m.overlay == "repairs" || m.overlay == "plans" {
			// These panels keep two rows under their content for the
			// reason input, as when they opened.
			infoHeight = max(1, infoHeight-2)
		}
		m.Info.SetWidth(infoWidth)
		m.Info.Viewport.SetHeight(infoHeight)
		m.Transcript.ApplyTheme(m.Theme)
		m = m.autoOpenIncidentCard()
		m = m.autoOpenRepairPanel()
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
			// An ordinary turn takes the queued message itself before this
			// request; a ceremony step is never steered, so it stops here.
			if m.Header.State.Ceremony != nil && m.steer.Peek() != "" && m.cancel != nil {
				m.steerCancelled = m.operationID
				m.record(application.DiagnosticEvent{Stage: application.DiagnosticSteerCancelled, Bytes: len(m.steer.Peek())})
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
		if v.Kind == application.EventToolCallProgress && m.Busy && m.streamPending {
			m.toolCallName = singleLine(v.ToolCallName)
			m.toolCallBytes = v.ToolCallBytes
		}
		if v.Kind == application.EventTextDelta || v.Kind == application.EventToolExecutionStarted {
			m.toolCallName, m.toolCallBytes = "", 0
		}
		if v.Kind == application.EventState {
			m.Status.State = v.State
		}
		if v.Usage != nil {
			m.tokens = v.Usage.TotalTokens
			m.cached = 0
			if v.Usage.PromptTokens > 0 {
				m.cached = 100 * v.Usage.CachedTokens / v.Usage.PromptTokens
			}
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
		// m.Info is shared by several overlays: a result is written only
		// while its own panel is open, never over one opened since.
		if v.MADEPreparation != nil && m.overlay == "made-setup" {
			m.Info.SetContent(madePreparationContent(*v.MADEPreparation, v.Err, m.Theme))
			m.Info.Viewport.GotoTop()
		}
		if v.EngineUpdates != nil && m.overlay == "updates" {
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
			m.Picker = NewSessionPicker(*v.Sessions, m.Header.State.Workspace)
			if m.deps.SessionLabels != nil {
				if labels, err := m.deps.SessionLabels.Load(m.lifetime.ctx); err == nil {
					m.Picker.Labels = labels
				}
			}
			m.overlay = "sessions"
		}
		if oldID != m.Header.State.ID {
			m.overlay = ""
			m.SearchBox = SearchBox{}
			m.tokens = 0
			m.cached = 0
			m.unsentPrompts = nil
			m.resetPromptHistory()
			m.draft = ""
			m.draftOperationID = 0
		}
		// returned says where a prompt the session did not take went; it
		// follows the operation's error, which cannot know.
		returned := ""
		if m.submittedPrompt != "" && v.Err != nil {
			messages := m.Header.State.Messages
			// StartTurn stores the prompt with console notes appended (a
			// resumed step, repair notices, the ceremony step), so the
			// stored message starts with the prompt rather than equals it.
			accepted := len(messages) > m.submittedAt && messages[m.submittedAt].Role == root.RoleUser && strings.HasPrefix(string(messages[m.submittedAt].Content), m.submittedPrompt)
			if !accepted {
				if m.Composer.Input.Value() == "" {
					m.Composer.Input.SetValue(m.submittedPrompt)
					returned = m.Theme.T("error.promptReturned")
				} else {
					m.unsentPrompts = append(m.unsentPrompts, m.submittedPrompt)
					returned = m.Theme.T("error.promptUnsent")
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
		steered := (m.steerCancelled == v.ID || m.closedCancelled == v.ID) && errors.Is(v.Err, context.Canceled)
		m.steerCancelled, m.closedCancelled = 0, 0
		if v.Err != nil && !steered {
			m.Status.Error = v.Err.Error()
			if v.PluginApproval != nil {
				m.Status.Error = m.Theme.T("error.approvalRefresh") + v.Err.Error()
			}
			if errors.Is(v.Err, domain.ErrToolCallLimit) {
				// The turn paused with its answer kept: say how it goes on,
				// in the person's language, and keep any error joined to it.
				m.Status.Error = strings.Replace(m.Status.Error, domain.ErrToolCallLimit.Error(), m.Theme.T("error.toolCallLimit"), 1)
			}
			if returned != "" {
				m.Status.Error += " " + returned
			}
		} else if m.draft == "" {
			m.Status.Error = ""
		}
		if v.ModelSelection && v.Err == nil && v.PreferenceErr != nil {
			m.Status.Error = m.Theme.T("error.modelDefault")
		}
		m.refreshTranscript()
		m.syncApproval()
		if m.steer.Peek() == "" && !m.inlineApproval() {
			m = m.autoOpenIncidentCard()
			m = m.autoOpenRepairPanel()
		}
		if queued, ok := m.steer.Take(); ok {
			prompt := queued
			m.refreshTranscript()
			m.record(application.DiagnosticEvent{Stage: application.DiagnosticInputSubmitted, OperationID: m.operationID + 1, Bytes: len(prompt), Messages: len(m.Header.State.Messages) + 1})
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
			if suggestions := slashSuggestions(command); len(suggestions) > 0 && m.overlay == "" {
				// Enter on a partial command runs the highlighted suggestion.
				command = suggestions[min(m.slashSelected, len(suggestions)-1)].name
			}
			command, _ = isSlashCommand(command)
			if command == "/exit" {
				if m.Busy {
					return m.Update(ControlIntent("cancel"))
				}
				m.zones.Close()
				return m, tea.Quit
			}
			if unknownSlashWord(command) {
				m.Status.Error = m.Theme.Tf("error.unknownCommand", command)
				return m, nil
			}
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
			if command == "/copy" {
				m.Composer.Input.Reset()
				return m, m.copyLatest()
			}
			if command == "/stop-ceremony" {
				m.Composer.Input.Reset()
				return m.stopCeremony()
			}
			if command == "/changes" || command == "/diff" {
				m.Composer.Input.Reset()
				return m.Update(ControlIntent("changes"))
			}
			if command == "/incident" || command == "/repair" || command == "/improve" || command == "/plan" {
				if _, awaiting := m.incidentRun(); awaiting {
					m.Composer.Input.Reset()
					return m.openIncidentCard(), nil
				}
			}
			if command == "/plan" && len(m.activePlans()) > 0 {
				// A plan that runs or was interrupted is shown first; n in
				// the panel starts a new one.
				m.Composer.Input.Reset()
				return m.openPlanPanel(), nil
			}
			if command == "/repair" && len(m.repairRecords()) > 0 || command == "/improve" && improvementRecords(m.repairRecords()) {
				// Repairs and improvements the agent requested from this
				// session, or that wait for the person, are shown before any
				// mode change; n in the panel then selects the mode.
				m.Composer.Input.Reset()
				m = m.openRepairPanel()
				m.RepairPanel.mode = slashModes[command]
				return m, nil
			}
			if mode, ok := slashModes[command]; ok {
				return m.switchMode(mode)
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
			if m.Busy && command != "" && m.turnRunning() {
				message := m.Composer.Input.Value()
				m.rememberPrompt(message)
				m.steer.Add(message)
				m.record(application.DiagnosticEvent{Stage: application.DiagnosticPromptQueued, Bytes: len(message)})
				m.Composer.Input.Reset()
				m.clearStaleError()
				m.refreshTranscript()
				// The queued message waits for the current model step: the turn
				// takes it after its next tool step, or it starts the next turn.
				return m, nil
			}
			if strings.TrimSpace(m.Composer.Input.Value()) == "" {
				return m, nil
			}
			if m.Busy {
				// An operation that is not a turn runs (an update, MADE
				// preparation, a list): the draft stays, and the person
				// learns why it was not sent.
				m.Status.Notice = m.Theme.T("notice.operationRunning")
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
		m.Status.Notice = ""
		if suggestions := slashSuggestions(strings.TrimSpace(m.Composer.Input.Value())); len(suggestions) > 0 && m.overlay == "" {
			switch v.String() {
			case "up":
				m.slashSelected = max(0, min(m.slashSelected, len(suggestions)-1)-1)
				return m, nil
			case "down":
				m.slashSelected = min(len(suggestions)-1, m.slashSelected+1)
				return m, nil
			case "tab":
				m.Composer.Input.SetValue(suggestions[min(m.slashSelected, len(suggestions)-1)].name)
				m.slashSelected = 0
				return m, nil
			}
		}
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
			if m.Transcript.Selection.Set {
				m.clearSelection()
				return m, nil
			}
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
			suggestions := slashSuggestions(strings.TrimSpace(m.Composer.Input.Value()))
			for i := range suggestions {
				if m.zones.Get(fmt.Sprintf("%sslash-%d", m.prefix, i)).InBounds(v) {
					m.slashSelected = i
					return m.Update(ControlIntent("send"))
				}
			}
			// A press on the conversation starts a selection that the
			// release copies; the terminal keeps Shift+drag for itself.
			m.beginSelection(v)
		}
		return m, nil
	case tea.MouseMotionMsg:
		m.extendSelection(v)
		return m, nil
	case tea.MouseReleaseMsg:
		return m, m.endSelection(v)
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.Transcript.Viewport, cmd = m.Transcript.Viewport.Update(v)
		return m, cmd
	}
	var cmd tea.Cmd
	before := m.Composer.Input.Value()
	m.Composer, cmd = m.Composer.Update(msg)
	if m.Composer.Input.Value() != before {
		m.slashSelected = 0
	}
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
		body := m.mainTranscript()
		if suggestions := slashSuggestions(strings.TrimSpace(m.Composer.Input.Value())); len(suggestions) > 0 && m.overlay == "" && !m.inlineApproval() {
			lines := strings.Split(body, "\n")
			menu := m.slashMenu(suggestions[:min(len(suggestions), len(lines))])
			body = strings.Join(append(lines[:len(lines)-len(menu)], menu...), "\n")
		}
		content = lipgloss.JoinVertical(lipgloss.Left, m.Header.View(m.Layout.Width, m.Theme), body, composer, footer)
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
			// Header, the conversation as drawn (a long error takes rows
			// from it) and the rule above the composer.
			view.Cursor.Y += 2 + m.Layout.BodyHeight - (m.footerRows() - 1)
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
	if !m.Transcript.Selection.Active {
		// New rows move the lines a finished selection pointed at.
		m.clearSelection()
	}
	m.Changes.SetSession(m.Header.State)
	m.Transcript.SetSession(m.Header.State, m.draft, m.Theme)
	if len(m.unsentPrompts) > 0 {
		m.Transcript.AppendUnsent(m.unsentPrompts)
	}
	m.Transcript.AppendQueued(m.steer.Peek())
}
