package terminal

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
)

func (m AppModel) pending() (domain.PendingTool, bool) {
	for _, p := range m.Header.State.Activity {
		if p.Outcome == nil {
			return p, true
		}
	}
	return domain.PendingTool{}, false
}
func (m AppModel) approvalFocus() bool {
	_, ok := m.pending()
	// A decision operation owns the pending call until it publishes a new
	// session. Its old approval snapshot must not obscure follow-up streaming.
	return !m.Busy && ok && m.Header.State.Status == domain.StatusApproval
}
func (m AppModel) knownPending() bool {
	p, ok := m.pending()
	if !ok {
		return false
	}
	_, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	return known && err == nil
}
func (m *AppModel) syncApproval() {
	p, ok := m.pending()
	if !ok {
		return
	}
	target := "unknown tool"
	if tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call); known && err == nil {
		switch tool.Identity.Kind {
		case domain.ToolKindLocal:
			target = fmt.Sprintf("local %s in %s", tool.Identity.LocalOperation, m.Header.State.Workspace)
		case domain.ToolKindPlugin:
			target = fmt.Sprintf("plugin %s / %s", tool.Identity.Plugin.PluginID, tool.Identity.Plugin.ToolName)
		case domain.ToolKindHost:
			target = fmt.Sprintf("read-only host %s", tool.Identity.LocalOperation)
		}
	}
	if m.Approval.Target != target || m.Approval.Pending.Call.ID != p.Call.ID || m.Approval.Pending.Call.Name != p.Call.Name || string(m.Approval.Pending.Call.Arguments.Bytes()) != string(p.Call.Arguments.Bytes()) {
		m.Approval = NewApprovalDialog(p, target)
	}

	m.sizeApproval()
}
func (m *AppModel) sizeApproval() {
	m.Approval.Details.Viewport.SetWidth(max(1, m.Layout.Width))
	m.Approval.Details.Viewport.SetHeight(max(1, m.Layout.Height-4))
}

// navigation routes modal input before editor input. It never reads a worker's
// session: all decisions use the UI's last published state.
func (m AppModel) navigation(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if m.overlay == "models" && !m.approvalFocus() {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseWheelMsg:
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
				return m, nil, false
			}
			var intent ControlIntent
			var cmd tea.Cmd
			m.Models, intent, cmd = m.Models.Update(msg, m.zones, m.prefix+"models-")
			if intent != "" {
				return m.navigation(intent)
			}
			return m, cmd, true
		}
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		if m.approvalFocus() {
			return m, nil, true
		}
		if m.overlay == "search" {
			var cmd tea.Cmd
			m.SearchBox.Input, cmd = m.SearchBox.Input.Update(paste)
			m.search()
			return m, cmd, true
		}
		if m.overlay != "" {
			return m, nil, true
		}
	}
	intent, hasIntent := msg.(ControlIntent)
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if k.String() == "ctrl+c" && !m.approvalFocus() {
			return m, nil, false
		}
		if m.approvalFocus() {
			intent = ControlIntent(m.Approval.Intent(k))
			if k.String() == "esc" || k.String() == "ctrl+c" {
				intent = "cancel"
			}
			if k.String() == "ctrl+r" && !m.knownPending() {
				intent = "continue"
			}
			hasIntent = intent != ""
			if !hasIntent {
				m.Approval.Details.Viewport, _ = m.Approval.Details.Viewport.Update(k)
				return m, nil, true
			}
		} else if m.overlay != "" {
			if k.String() == "esc" {
				intent = "close"
				hasIntent = true
			} else {
				switch m.overlay {
				case "mcp", "plugins":
					intent = m.Plugins.Update(k, m.overlay)
					hasIntent = intent != ""
				case "info":
					m.Info.Viewport, _ = m.Info.Viewport.Update(k)
				case "palette":
					intent = m.Palette.Intent(k)
					hasIntent = intent != ""
				case "sessions":
					switch k.String() {
					case "up":
						m.Picker.Selected = max(0, m.Picker.Selected-1)
					case "down":
						m.Picker.Selected = min(max(0, len(m.Picker.Items)-1), m.Picker.Selected+1)
					case "enter":
						intent = "open-session"
						hasIntent = true
					}
				case "search":
					switch k.String() {
					case "enter":
						intent = "next"
						hasIntent = true
					case "shift+enter":
						intent = "previous"
						hasIntent = true
					default:
						var cmd tea.Cmd
						m.SearchBox.Input, cmd = m.SearchBox.Input.Update(k)
						m.search()
						return m, cmd, true
					}
				}
				if !hasIntent {
					return m, nil, true
				}
			}
		} else {
			switch k.String() {
			case "ctrl+p":
				intent = "palette"
			case "ctrl+f":
				intent = "search"
			case "ctrl+o":
				intent = "sessions"
			case "f1":
				intent = "help"
			case "ctrl+r":
				intent = "continue"
			case "esc":
				intent = "cancel"
			}
			hasIntent = intent != ""
		}
	}
	if mouse, ok := msg.(tea.MouseClickMsg); ok && mouse.Button == tea.MouseLeft {
		ids := []string{"mcp", "plugins", "approve", "deny", "cancel", "models", "palette", "search", "sessions", "help", "info", "continue", "close", "previous", "next"}
		if m.overlay == "sessions" {
			for i := range m.Picker.Items {
				if m.zones.Get(fmt.Sprintf("%ssession-%d", m.prefix, i)).InBounds(mouse) {
					m.Picker.Selected = i
					intent = "open-session"
					hasIntent = true
					break
				}
			}
		}
		for _, id := range ids {
			if m.zones.Get(m.prefix + id).InBounds(mouse) {
				intent = ControlIntent(id)
				hasIntent = true
				break
			}
		}
		if !hasIntent && (m.approvalFocus() || m.overlay != "") {
			return m, nil, true
		}
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && (m.overlay == "mcp" || m.overlay == "plugins") {
		m.Plugins.Update(wheel, m.overlay)
		return m, nil, true
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.approvalFocus() {
		m.Approval.Details.Viewport, _ = m.Approval.Details.Viewport.Update(wheel)
		return m, nil, true
	}
	if !hasIntent {
		return m, nil, false
	}
	if m.approvalFocus() && intent != "approve" && intent != "deny" && intent != "cancel" && !(intent == "continue" && !m.knownPending()) {
		return m, nil, true
	}
	switch intent {
	case "mcp", "plugins", "plugins-refresh", "plugins-toggle":
		if m.Busy {
			m.Status.Error = "Wait for the current operation before managing plugins"
			return m, nil, true
		}
		if intent == "mcp" || intent == "plugins" {
			m.overlay = intent
		}
		manager := m.deps.Plugins
		var items []domain.PluginState
		toggle := intent == "plugins-toggle"
		if toggle && (m.overlay != "plugins" || len(m.Plugins.Items) == 0) {
			return m, nil, true
		}
		var id root.PluginID
		var selectedPolicy domain.PluginProfile
		policySaved := false
		mode := domain.ApprovalManual
		if toggle {
			selected := m.Plugins.Items[m.Plugins.Selected]
			id = selected.Profile.ID
			selectedPolicy = selected.Profile
			if selected.Profile.Approval == domain.ApprovalManual {
				mode = domain.ApprovalAuto
			}
		}
		selectedPolicy.Approval = mode
		m.Plugins.Loading = true
		m.Plugins.Error = ""
		cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
			if manager == nil {
				return errors.New("plugin management unavailable")
			}
			if toggle {
				if err := manager.SetApproval(ctx, id, mode); err != nil {
					return err
				}
				policySaved = true
			}
			var err error
			items, err = manager.List(ctx)
			return err
		})
		return m, func() tea.Msg {
			msg := cmd()
			if done, ok := msg.(operationComplete); ok {
				done.Plugins = &items
				if policySaved {
					done.PluginApproval = &selectedPolicy
				}
				return done
			}
			return msg
		}, true
	case "models", ModelRetryIntent:
		if m.Busy {
			m.Status.Error = "Cannot choose a model while an operation is running"
			return m, nil, true
		}
		if _, pending := m.pending(); pending {
			m.Status.Error = "Resolve pending tool calls before choosing a model"
			return m, nil, true
		}
		if m.Header.State.ID != "" && m.Header.State.Status != domain.StatusIdle && m.Header.State.Status != domain.StatusComplete && m.Header.State.Status != domain.StatusInterrupted {
			m.Status.Error = "Cannot choose a model while a turn is active"
			return m, nil, true
		}
		if intent == "models" {
			m.Models = NewModelPicker()
		}
		m.Models.pageSize = max(1, m.Layout.Height-7)
		m.Models.Input.SetWidth(max(1, m.Layout.Width-9))
		m.Models.SetLoading(true)
		m.overlay = "models"
		m.Status.Error = ""
		var models []domain.AvailableModel
		catalog := m.deps.Models
		cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
			var err error
			models, err = catalog.Execute(ctx)
			return err
		})
		return m, func() tea.Msg {
			msg := cmd()
			if done, ok := msg.(operationComplete); ok {
				done.Models = &models
				return done
			}
			return msg
		}, true
	case ModelSelectIntent:
		if m.Busy || m.overlay != "models" {
			return m, nil, true
		}
		selected, ok := m.Models.SelectedModel()
		if !ok {
			return m, nil, true
		}
		create, change, preference, id, workspace := m.deps.Create, m.deps.Change, m.deps.ModelPreference, m.deps.NewSessionID, m.deps.Workspace
		m.Models.SetLoading(true)
		m.Status.Error = ""
		var preferenceErr error
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
			var err error
			if s.Export().ID == "" {
				var created domain.Session
				created, err = create.Execute(ctx, id, workspace, selected.ID)
				if err == nil {
					*s = created
				}
			} else {
				err = change.Execute(ctx, s, selected.ID)
			}
			if err == nil && preference != nil {
				preferenceErr = preference.Save(ctx, selected.ID)
			}
			return err
		})
		return m, func() tea.Msg {
			msg := cmd()
			if done, ok := msg.(operationComplete); ok {
				done.ModelSelection = true
				done.PreferenceErr = preferenceErr
				return done
			}
			return msg
		}, true
	case ModelCloseIntent:
		if m.cancel != nil {
			m.cancel()
		}
		m.overlay = ""
		return m, nil, true
	case "approve", "deny":
		if m.Busy || m.Layout.TooSmall || !m.approvalFocus() || !m.knownPending() {
			return m, nil, true
		}
		p, _ := m.pending()
		decision := m.Approval.Intent(intent)
		resolve := m.deps.Resolve
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
			return resolve.Execute(ctx, s, p.Call.ID, decision, emit)
		})
		return m, cmd, true
	case "cancel":
		if m.cancel != nil {
			m.cancel()
			return m, nil, true
		}
		if m.Header.State.Status != domain.StatusApproval && m.Header.State.Status != domain.StatusInterrupted && m.Header.State.Status != domain.StatusStreaming {
			return m, nil, true
		}
		u := application.SessionControlUseCase{Store: m.deps.Store}
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
			return u.Cancel(ctx, s)
		})
		return m, cmd, true
	case "continue":
		if m.Busy || (m.Header.State.Status != domain.StatusInterrupted && m.Header.State.Status != domain.StatusApproval && m.Header.State.Status != domain.StatusStreaming) {
			return m, nil, true
		}
		m.overlay = ""
		u := application.SessionControlUseCase{Store: m.deps.Store, Agent: m.deps.Agent}
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
			return u.Resume(ctx, s, emit)
		})
		return m, cmd, true
	case "sessions":
		if m.Busy {
			return m, nil, true
		}
		store := m.deps.Store
		var summaries []domain.SessionSummary
		cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
			if store == nil {
				return errors.New("session store unavailable")
			}
			var err error
			summaries, err = store.List(ctx)
			return err
		})
		return m, func() tea.Msg {
			message := cmd()
			done, ok := message.(operationComplete)
			if !ok {
				// Shutdown can close the operation channel without publishing
				// completion when nobody is left to consume its events.
				return message
			}
			if done.Err == nil {
				done.Sessions = &summaries
			}
			return done
		}, true
	case "open-session":
		if m.Busy || len(m.Picker.Items) == 0 {
			return m, nil, true
		}
		id := m.Picker.Items[m.Picker.Selected].ID
		store := m.deps.Store
		workspace := m.Header.State.Workspace
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
			if store == nil {
				return errors.New("session store unavailable")
			}
			loaded, err := store.Load(ctx, id)
			if err != nil {
				return err
			}
			if loaded.Export().Workspace != workspace {
				return fmt.Errorf("session workspace %s differs from active workspace %s; reopen with that workspace", loaded.Export().Workspace, workspace)
			}
			*s = loaded
			return nil
		})
		return m, cmd, true
	case "palette", "help", "info":
		if intent == "info" {
			m.Info = NewTranscript()
			m.Info.Viewport.SetWidth(max(1, m.Layout.Width))
			m.Info.Viewport.SetHeight(max(1, m.Layout.Height-4))
			detail := fmt.Sprintf("Model: %s\nWorkspace: %s\nSession: %s\nSessions are local user data.\n\nFull tool results (saved history):", m.Header.State.Model, m.Header.State.Workspace, m.Header.State.ID)
			for _, record := range m.Header.State.Activity {
				if record.Outcome != nil {
					label, _ := toolPresentation(m.Header.State, record.Call.Name)
					detail += "\n\n" + label + " · " + string(record.Decision) + "\n" + string(record.Outcome.Content)
				}
			}
			m.Info.SetContent(detail)
			m.Info.Viewport.GotoTop()
		}
		m.overlay = intent
		return m, nil, true
	case "search":
		m.overlay = "search"
		m.SearchBox = NewSearchBox()
		m.SearchBox.Input.SetWidth(max(1, m.Layout.Width-18))
		return m, nil, true
	case "close":
		m.overlay = ""
		return m, nil, true
	case "next", "previous":
		if len(m.SearchBox.Hits) > 0 {
			delta := 1
			if intent == "previous" {
				delta = -1
			}
			m.SearchBox.Selected = (m.SearchBox.Selected + delta + len(m.SearchBox.Hits)) % len(m.SearchBox.Hits)
			m.showHit()
		}
		return m, nil, true
	}
	return m, nil, false
}
func (m *AppModel) search() { // Restore makes a private validated snapshot; no shared session access.
	s, err := domain.RestoreSession(m.Header.State)
	if err != nil {
		return
	}
	m.SearchBox.Hits = m.deps.Search.Execute(s, root.Text(m.SearchBox.Input.Value()))
	m.SearchBox.Selected = 0
	m.showHit()
}
func (m *AppModel) showHit() {
	if len(m.SearchBox.Hits) == 0 {
		return
	}
	hit := m.SearchBox.Hits[m.SearchBox.Selected]
	prefix := m.Header.State
	prefix.Draft = ""
	if hit.MessageIndex != nil {
		prefix.Messages = prefix.Messages[:*hit.MessageIndex]
	} else if hit.ArchivedDraftIndex != nil {
		archived := *hit.ArchivedDraftIndex
		prefix.Messages = prefix.Messages[:prefix.ArchivedDrafts[archived].AfterMessage]
		prefix.ArchivedDrafts = prefix.ArchivedDrafts[:archived]
	}
	rendered := NewTranscript()
	rendered.SetSession(prefix, "", Theme{Monochrome: true})
	before := rendered.Viewport.GetContent()
	lines := 0
	if before != "" {
		for _, line := range strings.Split(before, "\n") {
			lines += max(1, (ansi.StringWidth(line)+max(1, m.Layout.TranscriptWidth)-1)/max(1, m.Layout.TranscriptWidth))
		}
	}
	m.Transcript.Viewport.SetYOffset(lines)
}
func (m AppModel) overlayView(base string) string {
	status := m.statusView()
	if m.approvalFocus() {
		if m.knownPending() {
			return m.Approval.View(m.zones, m.prefix) + "\n" + status
		}
		return "Unknown tool request cannot be approved\n" + m.Approval.Details.View() + "\n" + m.zones.Mark(m.prefix+"continue", "[Reject and continue Ctrl+R]") + " " + m.zones.Mark(m.prefix+"cancel", "[Cancel Esc]") + "\n" + status
	}
	var body string
	switch m.overlay {
	case "mcp", "plugins":
		body = m.Plugins.View(m.overlay, m.Layout.Width, m.Layout.Height-2)
	case "models":
		body = m.Models.View(m.zones, m.prefix+"models-", m.Layout.Width, m.Layout.Height-2)
	case "palette":
		body = m.Palette.View(m.zones, m.prefix)
	case "help":
		body = m.Help.View(m.zones, m.prefix)
	case "info":
		body = "Model / workspace — ↑↓ / PgUp PgDn\n" + m.Info.View() + "\n" + m.zones.Mark(m.prefix+"close", "[Close Esc]")
	case "sessions":
		body = m.Picker.View(m.zones, m.prefix, m.Layout.Height-2, m.Layout.Width)
	case "search":
		body = m.Transcript.View() + "\n" + m.SearchBox.View(m.zones, m.prefix)
	}
	if body != "" {
		return body + "\n" + status
	}
	// Keep the original editor geometry while making navigation discoverable.
	nav := m.zones.Mark(m.prefix+"palette", "[Actions Ctrl+P]") + " " + m.zones.Mark(m.prefix+"help", "[Help F1]")
	if m.Header.State.Status == domain.StatusInterrupted || m.Header.State.Status == domain.StatusStreaming {
		nav += " " + m.zones.Mark(m.prefix+"continue", "[Continue Ctrl+R]")
	}
	return strings.Replace(base, status, nav+"\n"+status, 1)
}
