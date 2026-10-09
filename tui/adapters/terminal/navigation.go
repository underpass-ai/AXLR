package terminal

import (
	"charm.land/bubbles/v2/list"
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
	target := m.Theme.T("approval.unknownTarget")
	if tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call); known && err == nil {
		switch tool.Identity.Kind {
		case domain.ToolKindLocal:
			target = m.Theme.Tf("approval.localTarget", tool.Identity.LocalOperation, m.Header.State.Workspace)
		case domain.ToolKindPlugin:
			target = m.Theme.Tf("approval.pluginTarget", tool.Identity.Plugin.PluginID, tool.Identity.Plugin.ToolName)
		case domain.ToolKindHost:
			target = m.Theme.Tf("approval.hostTarget", tool.Identity.LocalOperation)
			if tool.Identity.LocalOperation == domain.HostOperationStepDone {
				target = m.Theme.T("approval.checkCommand")
			}
		}
	}
	if m.Approval.Target != target || m.Approval.Pending.Call.ID != p.Call.ID || m.Approval.Pending.Call.Name != p.Call.Name || string(m.Approval.Pending.Call.Arguments.Bytes()) != string(p.Call.Arguments.Bytes()) {
		m.Approval = NewApprovalDialog(p, target, m.Theme.Locale)
		if warning := m.hiddenInputWarning(p); warning != "" {
			m.Approval.Details.SetContent(warning + "\n\n" + m.Approval.Details.Text())
			m.Approval.Details.Viewport.GotoTop()
		}
	}

	m.sizeApproval()
}
func (m *AppModel) sizeApproval() {
	// A known call is decided in the inline card; an unknown one keeps the
	// full-screen dialog.
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Approval.Details.SetWidth(w)
	if m.knownPending() {
		h = m.approvalDetailRows()
	}
	m.Approval.Details.Viewport.SetHeight(h)
}

// navigation routes modal input before editor input. It never reads a worker's
// session: all decisions use the UI's last published state.
func (m AppModel) navigation(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if m.overlay == "jobs" && !m.approvalFocus() {
		if next, cmd, handled := m.jobsInput(msg); handled {
			return next, cmd, true
		}
	}
	if m.overlay == "changes" && !m.approvalFocus() {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseWheelMsg:
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
				return m, nil, false
			}
			if intent := m.Changes.Update(msg, m.zones, m.prefix+"changes-"); intent != "" {
				return m.navigation(intent)
			}
			return m, nil, true
		}
	}
	if m.overlay == "palette" && !m.approvalFocus() {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseWheelMsg, list.FilterMatchesMsg:
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
				return m, nil, false
			}
			var intent ControlIntent
			var cmd tea.Cmd
			m.Palette, intent, cmd = m.Palette.Update(msg)
			if intent != "" {
				return m.navigation(intent)
			}
			return m, cmd, true
		}
	}
	if m.overlay == "theme" && !m.approvalFocus() {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseWheelMsg:
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
				return m, nil, false
			}
			var intent ControlIntent
			var cmd tea.Cmd
			m.ThemePicker, intent, cmd = m.ThemePicker.Update(msg)
			switch intent {
			case "theme-preview":
				m.applyUIPreferences(m.ThemePicker.Preview)
			case "theme-cancel":
				m.applyUIPreferences(m.ThemePicker.Original)
				m.overlay = ""
			case "theme-save":
				if m.deps.UIPreferenceStore != nil {
					if err := m.deps.UIPreferenceStore.Save(m.lifetime.ctx, m.ThemePicker.Preview); err != nil {
						m.Status.Error = m.Theme.T("error.saveTheme") + err.Error()
						return m, nil, true
					}
				}
				m.UIPreferences = m.ThemePicker.Preview
				m.applyUIPreferences(m.UIPreferences)
				m.Status.Error = ""
				m.overlay = ""
			}
			return m, cmd, true
		}
	}
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
		if m.overlay == "mcp" {
			m.Plugins.Update(paste, m.overlay)
			return m, nil, true
		}
		if m.overlay == "plugins" {
			m.InstalledPlugins.Update(paste)
			return m, nil, true
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
		if k.String() == "esc" && m.overlay == "" && !m.approvalFocus() && m.Transcript.Selection.Set {
			// Esc first drops a copied selection; a second Esc cancels work.
			m.clearSelection()
			return m, nil, true
		}
		if m.approvalFocus() {
			intent = ControlIntent(m.Approval.Intent(k))
			if strings.ToLower(k.String()) == "f" {
				intent = "autonomy-on"
			}
			if intent == ControlIntent(domain.DecisionAutoApprove) {
				intent = "always-allow"
			}
			if (intent == "always-allow" || intent == "autonomy-on") && m.modeAsksForEachCall(m.Approval.Pending) {
				intent = "" // not offered: the mode keeps this call under approval
			}
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
			if m.overlay == "incident" && (k.String() != "esc" || m.IncidentCard.Reasoning) {
				next, cmd := m.incidentKey(k)
				return next, cmd, true
			}
			if m.overlay == "plans" && k.String() != "esc" {
				next, cmd := m.planKey(k)
				return next, cmd, true
			}
			if m.overlay == "repairs" && (k.String() != "esc" || m.RepairPanel.Reasoning) {
				next, cmd := m.repairKey(k)
				return next, cmd, true
			}
			if k.String() == "esc" {
				if m.overlay == "sessions" && m.Picker.Renaming {
					m.Picker.Key(k.String(), k.Text)
					return m, nil, true
				}
				if m.overlay == "mcp" && (m.Plugins.searching || m.Plugins.confirming || m.Plugins.installing) {
					m.Plugins.Update(k, m.overlay)
					return m, nil, true
				}
				if m.overlay == "plugins" && (m.InstalledPlugins.searching || m.InstalledPlugins.confirming || m.InstalledPlugins.addingMarketplace) {
					m.InstalledPlugins.Update(k)
					return m, nil, true
				}
				intent = "close"
				hasIntent = true
			} else {
				switch m.overlay {
				case "mcp":
					intent = m.Plugins.Update(k, m.overlay)
					hasIntent = intent != ""
				case "plugins":
					intent = m.InstalledPlugins.Update(k)
					hasIntent = intent != ""
				case "info", "approvals", "updates", "made-setup":
					m.Info.Viewport, _ = m.Info.Viewport.Update(k)
				case "sessions":
					if k.String() == "enter" && !m.Picker.Renaming {
						intent = "open-session"
						hasIntent = true
					} else if picked := m.Picker.Key(k.String(), k.Text); picked != "" {
						intent = picked
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
			case "ctrl+d":
				intent = "changes"
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
		ids := []string{"changes", "mcp", "plugins", "updates", "approve", "always-allow", "autonomy-on", "deny", "cancel", "models", "theme", "palette", "search", "sessions", "help", "info", "continue", "close", "previous", "next"}
		if m.overlay == "sessions" {
			for i := range m.Picker.Visible() {
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
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.overlay == "mcp" {
		m.Plugins.Update(wheel, m.overlay)
		return m, nil, true
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.overlay == "plugins" {
		m.InstalledPlugins.Update(wheel)
		return m, nil, true
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.approvalFocus() {
		m.Approval.Details.Viewport, _ = m.Approval.Details.Viewport.Update(wheel)
		return m, nil, true
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.overlay != "" && m.overlay != "search" {
		// The wheel scrolls what is on screen: the overlays drawn from
		// m.Info scroll it, and the others keep the conversation behind
		// them still. Search shows the conversation, so it falls through.
		switch m.overlay {
		case "info", "approvals", "updates", "made-setup", "plans", "repairs", "incident":
			m.Info.Viewport, _ = m.Info.Viewport.Update(wheel)
		}
		return m, nil, true
	}
	if !hasIntent {
		return m, nil, false
	}
	if m.approvalFocus() && intent != "approve" && intent != "always-allow" && intent != "autonomy-on" && intent != "deny" && intent != "cancel" && !(intent == "continue" && !m.knownPending()) {
		return m, nil, true
	}
	switch intent {
	case "updates":
		next, cmd := m.updateEngines()
		return next, cmd, true
	case "made-prepare":
		next, cmd := m.prepareMADE()
		return next, cmd, true
	case "changes":
		m.Changes.Open(m.Header.State)
		m.Changes.Resize(m.Layout.Width, m.Layout.Height-1)
		m.overlay = "changes"
		return m, nil, true
	case "theme":
		m.ThemePicker = NewThemePicker(m.UIPreferences, m.Theme.Locale)
		m.overlay = "theme"
		return m, nil, true
	case "copy":
		m.overlay = ""
		cmd := m.copyLatest()
		return m, cmd, true
	case "jobs":
		return m.openJobsPanel(), nil, true
	case "plugins", "catalog-refresh", "catalog-change", "catalog-marketplace":
		if m.Busy {
			m.Status.Error = m.Theme.T("error.pluginsBusy")
			return m, nil, true
		}
		if intent == "plugins" {
			m.overlay = "plugins"
			m.InstalledPlugins = NewInstalledPlugins()
			m.InstalledPlugins.Theme = m.Theme
			m.InstalledPlugins.Resize(m.Layout.Width, m.Layout.Height-2)
		}
		catalog := m.deps.InstalledPlugins
		change := intent == "catalog-change"
		marketplace := intent == "catalog-marketplace"
		marketplaceSource := strings.TrimSpace(m.InstalledPlugins.MarketplaceInput.Value())
		var selected application.InstalledPlugin
		if change {
			if m.overlay != "plugins" || len(m.InstalledPlugins.Items) == 0 {
				return m, nil, true
			}
			selected = m.InstalledPlugins.Items[m.InstalledPlugins.Selected]
			if selected.Installed || selected.Builtin {
				return m, nil, true
			}
		}
		if marketplace {
			m.InstalledPlugins.Available = true
		}
		available := m.InstalledPlugins.Available
		m.InstalledPlugins.Loading = true
		m.InstalledPlugins.Error = ""
		var items []application.InstalledPlugin
		cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
			if catalog == nil {
				return errors.New(m.Theme.T("catalog.unavailable"))
			}
			if marketplace {
				if err := catalog.AddSource(ctx, marketplaceSource); err != nil {
					return err
				}
			}
			if change {
				if err := catalog.Install(ctx, selected.ID); err != nil {
					return err
				}
			}
			var err error
			items, err = catalog.List(ctx, available)
			return err
		})
		return m, func() tea.Msg {
			msg := cmd()
			if done, ok := msg.(operationComplete); ok {
				done.InstalledPlugins = &items
				return done
			}
			return msg
		}, true
	case "mcp", "plugins-refresh", "plugins-toggle", "mcp-install":
		if m.Busy {
			m.Status.Error = m.Theme.T("error.pluginsBusy")
			return m, nil, true
		}
		if intent == "mcp" {
			m.overlay = intent
			m.Plugins = NewPluginPanel()
			m.Plugins.Theme = m.Theme
			m.Plugins.Search.Placeholder = m.Theme.T("plugins.searchPlaceholder")
			m.Plugins.Resize(m.Layout.Width, m.Layout.Height-2)
		}
		manager := m.deps.Plugins
		var items []domain.PluginState
		toggle := intent == "plugins-toggle"
		install := intent == "mcp-install"
		manifestPath := strings.TrimSpace(m.Plugins.InstallInput.Value())
		if toggle && (m.overlay != "mcp" || len(m.Plugins.Items) == 0) {
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
				return errors.New(m.Theme.T("error.pluginUnavailable"))
			}
			if install {
				if strings.HasPrefix(manifestPath, "/") {
					if err := manager.InstallManifest(ctx, manifestPath); err != nil {
						return err
					}
				} else {
					parts := strings.Fields(manifestPath)
					if len(parts) != 2 {
						return errors.New(m.Theme.T("mcp.installFormat"))
					}
					if err := manager.InstallURL(ctx, root.PluginID(parts[0]), parts[1]); err != nil {
						return err
					}
				}
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
			m.Status.Error = m.Theme.T("error.modelBusy")
			return m, nil, true
		}
		if _, pending := m.pending(); pending {
			m.Status.Error = m.Theme.T("error.modelPending")
			return m, nil, true
		}
		if m.Header.State.ID != "" && m.Header.State.Status != domain.StatusIdle && m.Header.State.Status != domain.StatusComplete && m.Header.State.Status != domain.StatusInterrupted {
			m.Status.Error = m.Theme.T("error.modelActive")
			return m, nil, true
		}
		if intent == "models" {
			m.Models = NewModelPicker()
			m.Models.Theme = m.Theme
			m.Models.SetCurrent(string(m.Header.State.Model))
			if m.deps.ModelFavorites != nil {
				if ids, err := m.deps.ModelFavorites.Load(m.lifetime.ctx); err == nil {
					m.Models.SetFavorites(modelIDStrings(ids))
				}
			}
			m.Models.Input.Prompt = m.Theme.T("common.searchPrompt")
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
	case ModelFavoriteIntent:
		selected, ok := m.Models.SelectedModel()
		if !ok || m.deps.ModelFavorites == nil {
			return m, nil, true
		}
		ids, err := m.deps.ModelFavorites.Toggle(m.lifetime.ctx, selected.ID)
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil, true
		}
		m.Status.Error = ""
		m.Models.SetFavorites(modelIDStrings(ids))
		return m, nil, true
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
			// The person closed the picker: the loading it stops is no
			// failure to report.
			m.closedCancelled = m.operationID
			m.cancel()
		}
		m.overlay = ""
		return m, nil, true
	case "approve", "always-allow", "autonomy-on", "deny":
		if m.Busy || m.Layout.TooSmall || !m.approvalFocus() || !m.knownPending() {
			return m, nil, true
		}
		p, _ := m.pending()
		decision := m.Approval.Intent(intent)
		resolve := m.deps.Resolve
		settings := m.deps.ApprovalSettings
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
			if intent == "autonomy-on" {
				if settings == nil {
					return errors.New(m.Theme.T("error.approvalSettings"))
				}
				if err := settings.SetAutonomous(ctx, true); err != nil {
					return err
				}
				decision = domain.DecisionApprove
			}
			if intent == "always-allow" {
				if settings == nil {
					return errors.New(m.Theme.T("error.approvalSettings"))
				}
				tool, _, known, err := application.ResolveToolCall(s.ToolSnapshot(), p.Call)
				if !known || err != nil {
					return errors.New(m.Theme.T("error.approvalUnknownTool"))
				}
				if err := settings.Allow(ctx, tool.Identity); err != nil {
					return err
				}
				decision = domain.DecisionApprove
			}
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
				return errors.New(m.Theme.T("error.sessionStoreUnavailable"))
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
	case SessionRenameIntent, SessionArchiveIntent:
		current, ok := m.Picker.Current()
		if !ok || m.deps.SessionLabels == nil {
			return m, nil, true
		}
		label := m.Picker.Labels[current.ID]
		if intent == SessionRenameIntent {
			label.Title = root.Text(strings.TrimSpace(m.Picker.RenameText))
		} else {
			label.Archived = !label.Archived
		}
		if err := m.deps.SessionLabels.Set(m.lifetime.ctx, current.ID, label); err != nil {
			m.Status.Error = err.Error()
			return m, nil, true
		}
		m.Status.Error = ""
		if m.Picker.Labels == nil {
			m.Picker.Labels = map[domain.SessionID]domain.SessionLabel{}
		}
		m.Picker.Labels[current.ID] = label
		m.Picker.Selected = min(m.Picker.Selected, max(0, len(m.Picker.Visible())-1))
		return m, nil, true
	case "open-session":
		selected, ok := m.Picker.Current()
		if m.Busy || !ok {
			return m, nil, true
		}
		id := selected.ID
		store := m.deps.Store
		workspace := m.Header.State.Workspace
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
			if store == nil {
				return errors.New(m.Theme.T("error.sessionStoreUnavailable"))
			}
			loaded, err := store.Load(ctx, id)
			if err != nil {
				return err
			}
			if loaded.Export().Workspace != workspace {
				return errors.New(m.Theme.Tf("error.workspaceMismatch", shortenHome(string(loaded.Export().Workspace)), shortenHome(string(workspace))))
			}
			*s = loaded
			return nil
		})
		return m, cmd, true
	case "palette":
		m.Palette = NewActionPalette(m.Theme.Locale)
		m.overlay = intent
		return m, nil, true
	case "help", "info", "approvals":
		if intent == "info" {
			m.Info = NewTranscript()
			w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
			m.Info.SetWidth(w)
			m.Info.Viewport.SetHeight(h)
			m.Info.SetContent(infoContentLocale(m.Header.State, m.Theme.Locale))
			m.Info.Viewport.GotoTop()
		} else if intent == "approvals" {
			m.Info = NewTranscript()
			w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
			m.Info.SetWidth(w)
			m.Info.Viewport.SetHeight(h)
			m.Info.SetContent(approvalContent(m.deps.ApprovalSettings, m.Theme.Locale))
			m.Info.Viewport.GotoTop()
		}
		m.overlay = intent
		return m, nil, true
	case "search":
		m.overlay = "search"
		m.SearchBox = NewSearchBox(m.Theme.Locale)
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
	// The prefix is measured with the transcript's own theme: Editorial
	// speaker rows, folded tool runs and the icon set change the line count.
	rendered := NewTranscript()
	rendered.SetWidth(m.Transcript.Viewport.Width())
	rendered.SetSession(prefix, "", m.Theme)
	m.Transcript.Viewport.SetYOffset(rendered.VisualLineCount())
}
func (m AppModel) overlayView(base string) string {
	status := m.statusView()
	if m.approvalFocus() {
		if m.knownPending() {
			return base
		}
		footer := m.zones.Mark(m.prefix+"continue", "["+m.Theme.T("approval.rejectContinue")+"]") + "  " + m.zones.Mark(m.prefix+"cancel", "["+m.Theme.T("common.cancel")+"]")
		return m.Theme.Overlay(m.Theme.T("approval.unknownTitle"), m.Theme.T("approval.unknownSubtitle")+" · "+string(m.Approval.Pending.Call.Name), m.Approval.Details.View(), footer, m.Layout.Width, m.Layout.Height-1) + "\n" + status
	}
	var body string
	switch m.overlay {
	case "changes":
		body = m.Changes.View(m.zones, m.prefix+"changes-", m.Layout.Width, m.Layout.Height-1)
	case "mcp":
		body = m.Plugins.View(m.overlay, m.Layout.Width, m.Layout.Height-2)
	case "plugins":
		body = m.InstalledPlugins.View(m.Layout.Width, m.Layout.Height-2)
	case "theme":
		body = m.ThemePicker.View(m.Theme, m.Layout.Width, m.Layout.Height-2)
	case "models":
		body = m.Models.View(m.zones, m.prefix+"models-", m.Layout.Width, m.Layout.Height-2)
	case "palette":
		body = m.Palette.View(m.Theme, m.zones, m.prefix, m.Layout.Width, m.Layout.Height-1)
	case "help":
		body = m.Help.View(m.Theme, m.zones, m.prefix, m.Layout.Width, m.Layout.Height-1)
	case "info", "approvals", "updates", "made-setup":
		title, subtitle := m.Theme.T("info.title"), m.Theme.T("info.subtitle")
		if m.overlay == "made-setup" {
			title, subtitle = m.Theme.T("madeSetup.title"), m.Theme.T("madeSetup.subtitle")
		}
		if m.overlay == "approvals" {
			title, subtitle = m.Theme.T("approvals.title"), m.Theme.T("approvals.subtitle")
		}
		if m.overlay == "updates" {
			title, subtitle = m.Theme.T("update.title"), m.Theme.T("update.subtitle")
		}
		body = m.Theme.Overlay(title, subtitle, m.Info.View(), m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), m.Layout.Width, m.Layout.Height-1)
	case "incident":
		title, subtitle, content := m.incidentCardView()
		body = m.Theme.Overlay(title, subtitle, content, m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), m.Layout.Width, m.Layout.Height-1)
	case "plans":
		body = m.Theme.Overlay(m.Theme.T("plans.title"), m.Theme.T("plans.hints"), m.Info.View(), m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), m.Layout.Width, m.Layout.Height-1)
	case "repairs":
		title, subtitle, content := m.repairPanelView()
		body = m.Theme.Overlay(title, subtitle, content, m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), m.Layout.Width, m.Layout.Height-1)
	case "jobs":
		body = m.jobsPanelView()
	case "sessions":
		body = m.Picker.View(m.Theme, m.zones, m.prefix, m.Layout.Height-1, m.Layout.Width)
	case "search":
		// Search lives in the main view, in place of the composer.
		return base
	}
	if body != "" {
		return fitOverlay(body, m.Layout.Width, m.Layout.Height-1) + "\n" + status
	}
	return base
}

func fitOverlay(body string, width, height int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = append(lines[:height-1], lines[len(lines)-1])
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, width), "…")
	}
	return strings.Join(lines, "\n")
}

func modelIDStrings(ids []root.ModelID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
