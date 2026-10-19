package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /tools (decision D4 of the tool registry design): alone it lists the
// workspace's tools and the catalogs'; search finds catalog tools; install
// stages one, shows the install card and installs it on y; update lists
// the installed catalog tools a catalog has newer and installs each on its
// own card; remove uninstalls one, as axlr_remove_tool does. The model's
// axlr_install_tool call is staged here too, for its approval card.

// toolsOutcome is what a /tools operation hands back: the panel's text,
// a staged tool waiting for y, a tool installed or removed.
type toolsOutcome struct {
	content   string
	staged    *application.StagedTool
	installed *application.ForgedTool
	replaced  bool
	removed   string
	// queue is what /tools update still has to install after this one.
	queue []string
	// keyCreated reports /tools keygen; notice is a status line to show.
	keyCreated bool
	notice     string
	// publishing marks a /tools publish stage, whose failure drops the
	// run; another verb's failure leaves a run that is polling alone.
	publishing bool
	// draft, bundle, report and retryAfter are /tools publish's stages
	// (tools_publish.go).
	draft      *application.PublishDraft
	bundle     *application.PublishBundle
	report     *application.VersionReport
	retryAfter time.Duration
}

// installStagedMsg is the model's pending axlr_install_tool call staged
// for its card.
type installStagedMsg struct {
	call   root.ToolCallID
	staged application.StagedTool
	err    error
}

// toolsCommand runs /tools with what followed it.
func (m AppModel) toolsCommand(args string) (tea.Model, tea.Cmd) {
	if !m.deps.ToolCatalog.Enabled() {
		m.Status.Error = m.Theme.T("tools.unavailable")
		return m, nil
	}
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		m.Composer.Input.Reset()
		return m.openToolsPanel(false, nil)
	}
	switch strings.ToLower(fields[0]) {
	case "search":
		if len(fields) < 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
			return m, nil
		}
		m.Composer.Input.Reset()
		return m.openToolsPanel(false, fields[1:])
	case "install":
		if len(fields) < 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
			return m, nil
		}
		request, err := parseInstallArguments(fields[1:])
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		m.Composer.Input.Reset()
		m.toolsQueue = nil
		return m.stageTool(request, nil)
	case "update":
		m.Composer.Input.Reset()
		name := ""
		if len(fields) > 1 {
			name = fields[1]
		}
		return m.updateTools(name)
	case "remove":
		if len(fields) != 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
			return m, nil
		}
		m.Composer.Input.Reset()
		return m.removeTool(fields[1])
	case "keygen", "identity":
		if len(fields) != 1 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
			return m, nil
		}
		m.Composer.Input.Reset()
		return m.showIdentity(fields[0] == "keygen")
	case "enroll":
		m.Composer.Input.Reset()
		return m.enrollPublisher(fields[1:])
	case "publish":
		if len(fields) < 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
			return m, nil
		}
		request, err := parsePublishArguments(fields[1:])
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		m.Composer.Input.Reset()
		return m.preparePublish(request)
	}
	m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
	return m, nil
}

// parseInstallArguments reads "<name>[@version] [--from <catalog>]".
func parseInstallArguments(fields []string) (application.InstallRequest, error) {
	request := application.InstallRequest{}
	request.Name, request.Version, _ = strings.Cut(fields[0], "@")
	for i := 1; i < len(fields); i++ {
		switch {
		case fields[i] == "--from" && i+1 < len(fields):
			request.Catalog = fields[i+1]
			i++
		case strings.HasPrefix(fields[i], "--from="):
			request.Catalog = strings.TrimPrefix(fields[i], "--from=")
		default:
			return request, fmt.Errorf("usage: %s", slashUsage("/tools"))
		}
	}
	return request, nil
}

// openToolsPanel shows the overview, or the search for terms, read in an
// operation.
func (m AppModel) openToolsPanel(refresh bool, terms []string) (AppModel, tea.Cmd) {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.T("tools.loading"))
	m.Info.Viewport.GotoTop()
	m.overlay = "tools"
	m.Status.Error = ""
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	theme := m.Theme
	var outcome toolsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if len(terms) > 0 {
			matches, catalogs, err := use.Search(ctx, workspace, terms)
			if err != nil {
				return err
			}
			outcome.content = toolsSearchContent(terms, matches, catalogs, theme)
			return nil
		}
		overview, err := use.Overview(ctx, workspace, refresh)
		if err != nil {
			return err
		}
		outcome.content = toolsOverviewContent(overview, theme)
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// toolsCompletion attaches the outcome to the operation's completion.
func toolsCompletion(cmd tea.Cmd, outcome *toolsOutcome) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.Tools = outcome
			return done
		}
		return msg
	}
}

// stageTool fetches a tool for its card; queue is what follows it.
func (m AppModel) stageTool(request application.InstallRequest, queue []string) (AppModel, tea.Cmd) {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.Tf("tools.staging", request.Name))
	m.Info.Viewport.GotoTop()
	m.overlay = "tools"
	m.Status.Error = ""
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	outcome := toolsOutcome{queue: queue}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		staged, err := use.Stage(ctx, workspace, request)
		if err != nil {
			return err
		}
		outcome.staged = &staged
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// installStagedTool writes the staged tool after y.
func (m AppModel) installStagedTool() (AppModel, tea.Cmd) {
	staged := m.toolsStaged
	if staged == nil {
		return m, nil
	}
	m.toolsStaged = nil
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	session := string(m.Header.State.ID)
	outcome := toolsOutcome{queue: m.toolsQueue}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		installed, replaced, err := use.Install(ctx, workspace, *staged, session)
		if err != nil {
			return err
		}
		outcome.installed, outcome.replaced = &installed, replaced
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// updateTools refreshes the catalogs and stages the installed tools they
// have newer, one card at a time; name limits it to one tool.
func (m AppModel) updateTools(name string) (AppModel, tea.Cmd) {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.T("tools.loading"))
	m.Info.Viewport.GotoTop()
	m.overlay = "tools"
	m.Status.Error = ""
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	theme := m.Theme
	var outcome toolsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		overview, err := use.Overview(ctx, workspace, true)
		if err != nil {
			return err
		}
		var names []string
		for _, update := range overview.Updates {
			if name == "" || update.Name == name {
				names = append(names, update.Name)
			}
		}
		if len(names) == 0 {
			if name != "" {
				for _, tool := range overview.Installed {
					if tool.Name == name && tool.Origin == nil {
						return fmt.Errorf("%s", theme.Tf("tools.notFromCatalog", name))
					}
				}
			}
			outcome.content = toolsOverviewContent(overview, theme) + "\n" + theme.T("tools.current")
			return nil
		}
		staged, err := use.Stage(ctx, workspace, application.InstallRequest{Name: names[0]})
		if err != nil {
			return err
		}
		outcome.staged, outcome.queue = &staged, names[1:]
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// removeTool uninstalls a tool by name.
func (m AppModel) removeTool(name string) (AppModel, tea.Cmd) {
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	outcome := toolsOutcome{}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		found, err := use.Forge.Remove(ctx, workspace, name)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%s", m.Theme.Tf("tools.unknown", name))
		}
		outcome.removed = name
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// completeTools applies a /tools operation's outcome.
func (m AppModel) completeTools(outcome toolsOutcome, err error) (AppModel, tea.Cmd) {
	if m.overlay == "tools" {
		switch {
		case err != nil:
			m.Info.SetContent(err.Error())
		case outcome.content != "":
			m.Info.SetContent(outcome.content)
		}
		m.Info.Viewport.GotoTop()
	}
	if err != nil {
		m.toolsQueue = nil
		if outcome.publishing {
			m.publishDraft, m.publishBundle, m.publishAsking = nil, nil, ""
		}
		if m.overlay != "tools" {
			m.Status.Error = err.Error()
		}
		return m, nil
	}
	if outcome.keyCreated {
		m.Status.Notice = m.Theme.T("notice.keyCreated")
		return m, nil
	}
	if outcome.notice != "" {
		m.Status.Notice = outcome.notice
		return m, nil
	}
	if outcome.draft != nil || outcome.bundle != nil || outcome.report != nil || outcome.retryAfter > 0 {
		return m.completePublish(outcome)
	}
	if outcome.removed != "" {
		m.Status.Notice = m.Theme.Tf("notice.toolRemoved", outcome.removed)
		return m, nil
	}
	if outcome.installed != nil {
		origin := outcome.installed.Origin
		if origin != nil {
			m.Status.Notice = m.Theme.Tf("notice.toolInstalled", outcome.installed.Name, origin.Version, origin.Catalog)
		}
		if m.overlay == "tools" {
			m.Info.SetContent(m.Theme.Tf("tools.installedPanel", outcome.installed.Name))
		}
		m.toolsQueue = outcome.queue
		if len(m.toolsQueue) > 0 {
			next := m.toolsQueue[0]
			m.toolsQueue = m.toolsQueue[1:]
			return m.stageTool(application.InstallRequest{Name: next}, m.toolsQueue)
		}
		return m, nil
	}
	if outcome.staged != nil {
		m.toolsStaged = outcome.staged
		m.toolsQueue = outcome.queue
		m.toolsAsking = true
		if m.overlay == "tools" {
			m.Info.SetContent(installCard(*outcome.staged, m.Theme))
			m.Info.Viewport.GotoTop()
		}
		m.Status.Error = ""
		m.Status.Notice = m.Theme.Tf("tools.confirm", outcome.staged.Entry.Name, outcome.staged.Entry.Version, outcome.staged.Catalog)
	}
	return m, nil
}

// answerTools takes the key that answers the install question.
func (m AppModel) answerTools(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	switch k.String() {
	case "up", "down", "pgup", "pgdown", "home", "end", "ctrl+home", "ctrl+end", "ctrl+u", "ctrl+d", "ctrl+f", "ctrl+b":
		// The card is read before it is answered: a multi-file tool
		// scrolls below the fold.
		if m.overlay == "tools" {
			if !jumpViewport(&m.Info.Viewport, k.String()) {
				m.Info.Viewport, _ = m.Info.Viewport.Update(k)
			}
		}
		return m, nil
	}
	m.toolsAsking = false
	publishing := m.publishBundle != nil
	if k.String() != "y" {
		m.toolsStaged, m.toolsQueue = nil, nil
		m.Status.Notice = m.Theme.T("tools.kept")
		if publishing {
			m.publishBundle, m.publishDraft = nil, nil
			m.Status.Notice = m.Theme.T("tools.publishKept")
		}
		if m.overlay == "tools" {
			m.overlay = ""
		}
		return m, nil
	}
	if m.Busy {
		m.Status.Notice = m.Theme.T("mode.busy")
		return m, nil
	}
	m.Status.Notice = ""
	if publishing {
		return m.uploadPublishBundle()
	}
	return m.installStagedTool()
}

// closeTools forgets the staged tool and its question when the panel
// closes by a click, so the next character typed is not its answer.
func (m *AppModel) closeTools() {
	if m.overlay == "tools" && (m.toolsAsking || m.publishAsking != "") {
		m.toolsAsking, m.toolsStaged, m.toolsQueue = false, nil, nil
		m.Status.Notice = m.Theme.T("tools.kept")
		m.closePublish()
	}
}

// toolsOverviewContent renders /tools: the workspace's tools, the updates
// and each catalog's tools.
func toolsOverviewContent(overview application.CatalogOverview, theme Theme) string {
	var b strings.Builder
	b.WriteString(theme.T("tools.installedTitle") + "\n")
	if len(overview.Installed) == 0 {
		b.WriteString("  " + theme.T("tools.noneInstalled") + "\n")
	}
	for _, tool := range overview.Installed {
		origin := theme.T("tools.forgedHere")
		if tool.Origin != nil {
			origin = theme.Tf("tools.fromCatalog", tool.Origin.Catalog, tool.Origin.Version)
		}
		fmt.Fprintf(&b, "  %s · %s\n    %s\n", tool.Name, origin, singleLine(tool.Description))
	}
	if len(overview.Updates) > 0 {
		b.WriteString("\n" + theme.T("tools.updatesTitle") + "\n")
		for _, update := range overview.Updates {
			fmt.Fprintf(&b, "  %s · %s → %s · %s\n", update.Name, update.Installed, update.Available, update.Catalog)
		}
	}
	for _, state := range overview.Catalogs {
		fmt.Fprintf(&b, "\n%s\n", theme.Tf("tools.catalogTitle", state.Name))
		if state.Warning != "" {
			fmt.Fprintf(&b, "  ▲ %s\n", singleLine(state.Warning))
		}
		if state.Index == nil {
			continue
		}
		for _, match := range application.NewestOfEach(state.Index.Tools) {
			fmt.Fprintf(&b, "  %s %s · %s\n    %s\n", match.Name, match.Version, match.Category, singleLine(match.Summary))
		}
	}
	b.WriteString("\n" + theme.T("tools.hint"))
	return b.String()
}

// toolsSearchContent renders /tools search.
func toolsSearchContent(terms []string, matches []application.CatalogMatch, catalogs []application.CatalogState, theme Theme) string {
	var b strings.Builder
	b.WriteString(theme.Tf("tools.searchTitle", strings.Join(terms, " ")) + "\n")
	if len(matches) == 0 {
		b.WriteString("  " + theme.T("tools.noMatch") + "\n")
	}
	for _, match := range matches {
		installed := ""
		if match.InstalledVersion != "" {
			installed = " · " + theme.Tf("tools.installedVersion", match.InstalledVersion)
		}
		fmt.Fprintf(&b, "  %s %s · %s · %s%s\n    %s\n", match.Tool.Name, match.Tool.Version, match.Catalog, match.Tool.Category, installed, singleLine(match.Tool.Summary))
	}
	for _, state := range catalogs {
		if state.Warning != "" {
			fmt.Fprintf(&b, "\n▲ %s\n", singleLine(state.Warning))
		}
	}
	b.WriteString("\n" + theme.T("tools.hint"))
	return b.String()
}

// installCard is the forge card with the origin line and the warnings:
// what the person reads before the files are written, from /tools install
// and from the model's axlr_install_tool alike.
func installCard(staged application.StagedTool, theme Theme) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s · %s %s · %s\n", theme.T("tools.originCatalog"), staged.Catalog, staged.Entry.Name, staged.Entry.Version, theme.T("tools.verified"))
	if staged.Entry.License != "" {
		fmt.Fprintf(&b, "%s %s\n", theme.T("tools.license"), staged.Entry.License)
	}
	for _, warning := range staged.Warnings {
		fmt.Fprintf(&b, "▲ %s\n", singleLine(warning))
	}
	b.WriteString("\n")
	b.WriteString(forgeCardOf(staged.Tool.Program, staged.Tool.Args, staged.Tool.Description, staged.Tool.InputSchema, staged.Files))
	return b.String()
}

// pendingInstall is the model's pending axlr_install_tool call, if that is
// what waits.
func (m AppModel) pendingInstall() (domain.PendingTool, application.InstallRequest, bool) {
	p, ok := m.pending()
	if !ok || p.Call.Name != application.HostInstallToolName {
		return domain.PendingTool{}, application.InstallRequest{}, false
	}
	request, ok := application.InstallRequestOf(p.Call.Arguments.Bytes())
	return p, request, ok
}

// installCardReady reports whether the pending install's card shows its
// files: approving before that would approve code unseen.
func (m AppModel) installCardReady() bool {
	p, _, ok := m.pendingInstall()
	if !ok {
		return true
	}
	return m.installStaged != nil && m.installStagedCall == p.Call.ID
}

// shownInstall is the staged tool the pending install's card shows, for
// the approval to install exactly that (application.WithInstallCard); ok
// is false when no install waits or its card shows no files yet.
func (m AppModel) shownInstall() (root.ToolCallID, application.StagedTool, bool) {
	p, _, ok := m.pendingInstall()
	if !ok || m.installStaged == nil || m.installStagedCall != p.Call.ID {
		return "", application.StagedTool{}, false
	}
	return p.Call.ID, *m.installStaged, true
}

// withShownInstall carries the card's staged tool, if one is shown, to
// the operation that resolves the call.
func withShownInstall(ctx context.Context, call root.ToolCallID, staged application.StagedTool, shown bool) context.Context {
	if !shown {
		return ctx
	}
	return application.WithInstallCard(ctx, call, staged)
}

// stageInstallCard fetches the pending install's files in the background;
// the completion fills the card. nil when nothing is pending or it is
// already staged or staging.
func (m *AppModel) stageInstallCard() tea.Cmd {
	p, request, ok := m.pendingInstall()
	if !ok || !m.deps.ToolCatalog.Enabled() || m.installStagedCall == p.Call.ID {
		return nil
	}
	m.installStagedCall, m.installStaged, m.installStageErr = p.Call.ID, nil, nil
	m.applyInstallCard()
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	ctx := m.lifetime.ctx
	call := p.Call.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		staged, err := use.Stage(ctx, workspace, request)
		return installStagedMsg{call: call, staged: staged, err: err}
	}
}

// applyInstallCard writes the pending install's card: the files once
// staged, the failure, or that they are being fetched.
func (m *AppModel) applyInstallCard() {
	p, request, ok := m.pendingInstall()
	if !ok || m.Approval.Pending.Call.ID != p.Call.ID {
		return
	}
	switch {
	case m.installStaged != nil && m.installStagedCall == p.Call.ID:
		m.Approval.Details.SetContent(installCard(*m.installStaged, m.Theme))
	case m.installStageErr != nil && m.installStagedCall == p.Call.ID:
		m.Approval.Details.SetContent(m.Theme.Tf("approval.installFailed", m.installStageErr.Error()))
	default:
		m.Approval.Details.SetContent(m.Theme.Tf("approval.installFetching", request.Name))
	}
	m.Approval.Details.Viewport.GotoTop()
}
