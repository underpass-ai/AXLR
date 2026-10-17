package terminal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /tools keygen, identity, enroll and publish are the console's side of
// the registry's client identity (axlr-registry docs/client-identity.md
// §5): one key per console, signed requests, self-service enrolment and
// the upload of a tool forged here. Publishing asks for the catalog
// fields catalog.json lacks in the composer, one at a time, shows the
// bundle on a card, uploads on y and then polls the version every 5 s
// for up to 4 minutes, showing the scan findings and the test result.

// publishPollMsg is one answer of the poll that follows an upload; gen
// tells a stale poll from the current one.
type publishPollMsg struct {
	gen    uint64
	report application.VersionReport
	err    error
}

// publishRetryMsg fires when a rate-limited upload is tried again.
type publishRetryMsg struct{ gen uint64 }

// publishFields are asked in this order, each with its label.
var publishFieldLabels = map[string]string{"version": "tools.fieldVersion", "category": "tools.fieldCategory", "license": "tools.fieldLicense", "tags": "tools.fieldTags"}

// showIdentity runs /tools keygen (create) or /tools identity.
func (m AppModel) showIdentity(create bool) (AppModel, tea.Cmd) {
	if !m.deps.ToolCatalog.Publishable() {
		m.Status.Error = m.Theme.T("tools.noRegistry")
		return m, nil
	}
	m = m.openToolsOverlay(m.Theme.T("tools.loading"))
	use := m.deps.ToolCatalog
	theme := m.Theme
	var outcome toolsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		if create {
			identity, err := use.Keygen(ctx)
			if err != nil {
				return err
			}
			outcome.content = identityContent(identity, true, theme)
			outcome.keyCreated = true
			return nil
		}
		identity, found, err := use.Identity(ctx)
		if err != nil {
			return err
		}
		if !found {
			outcome.content = theme.T("tools.noKey")
			return nil
		}
		outcome.content = identityContent(identity, false, theme)
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// identityContent renders the client key's public side.
func identityContent(identity application.ClientIdentity, created bool, theme Theme) string {
	var b strings.Builder
	b.WriteString(theme.T("tools.identityTitle") + "\n")
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.keyID"), identity.KeyID)
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.publicKey"), identity.PublicKey)
	if created {
		b.WriteString("  " + theme.T("tools.keyCreatedNow") + "\n")
	}
	b.WriteString("\n" + theme.T("tools.identityHint"))
	return b.String()
}

// enrollPublisher runs /tools enroll <publisher id> <namespace>… [--to
// <catalog>] [--contact <address>].
func (m AppModel) enrollPublisher(fields []string) (AppModel, tea.Cmd) {
	if !m.deps.ToolCatalog.Publishable() {
		m.Status.Error = m.Theme.T("tools.noRegistry")
		return m, nil
	}
	request, catalog, err := parseEnrollArguments(fields)
	if err != nil {
		m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/tools"))
		return m, nil
	}
	m = m.openToolsOverlay(m.Theme.Tf("tools.enrolling", request.ID))
	use := m.deps.ToolCatalog
	theme := m.Theme
	var outcome toolsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		result, name, err := use.Enroll(ctx, catalog, request)
		if err != nil {
			var limited *application.RateLimited
			if errors.As(err, &limited) {
				return fmt.Errorf("%s", theme.Tf("tools.rateLimited", limited.RetryAfter.Round(time.Second)))
			}
			return err
		}
		identity, _, _ := use.Identity(ctx)
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", theme.Tf("tools.enrollPending", name))
		fmt.Fprintf(&b, "  %s %s · %s %s\n", theme.T("tools.publisher"), result.ID, theme.T("tools.keyID"), identity.KeyID)
		fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.namespaces"), strings.Join(request.Namespaces, " "))
		outcome.content = b.String()
		outcome.notice = theme.Tf("tools.enrollPending", name)
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// parseEnrollArguments reads "<publisher id> <namespace>… --contact
// <address> [--to <catalog>]"; the registry requires the contact.
func parseEnrollArguments(fields []string) (application.EnrollRequest, string, error) {
	var request application.EnrollRequest
	catalog := ""
	for i := 0; i < len(fields); i++ {
		switch {
		case fields[i] == "--to" && i+1 < len(fields):
			catalog = fields[i+1]
			i++
		case strings.HasPrefix(fields[i], "--to="):
			catalog = strings.TrimPrefix(fields[i], "--to=")
		case fields[i] == "--contact" && i+1 < len(fields):
			request.Contact = fields[i+1]
			i++
		case strings.HasPrefix(fields[i], "--contact="):
			request.Contact = strings.TrimPrefix(fields[i], "--contact=")
		case strings.HasPrefix(fields[i], "--"):
			return request, "", fmt.Errorf("usage: %s", slashUsage("/tools"))
		case request.ID == "":
			request.ID = fields[i]
		default:
			request.Namespaces = append(request.Namespaces, fields[i])
		}
	}
	if request.ID == "" || len(request.Namespaces) == 0 || request.Contact == "" {
		return request, "", fmt.Errorf("usage: %s", slashUsage("/tools"))
	}
	return request, catalog, nil
}

// parsePublishArguments reads "<name>[@version] [--to <catalog>]".
func parsePublishArguments(fields []string) (application.PublishRequest, error) {
	request := application.PublishRequest{}
	request.Name, request.Version, _ = strings.Cut(fields[0], "@")
	for i := 1; i < len(fields); i++ {
		switch {
		case fields[i] == "--to" && i+1 < len(fields):
			request.Catalog = fields[i+1]
			i++
		case strings.HasPrefix(fields[i], "--to="):
			request.Catalog = strings.TrimPrefix(fields[i], "--to=")
		default:
			return request, fmt.Errorf("usage: %s", slashUsage("/tools"))
		}
	}
	return request, nil
}

// preparePublish runs /tools publish: the tool is read for its draft.
func (m AppModel) preparePublish(request application.PublishRequest) (AppModel, tea.Cmd) {
	if !m.deps.ToolCatalog.Publishable() {
		m.Status.Error = m.Theme.T("tools.noRegistry")
		return m, nil
	}
	m.publishGen++
	m.publishDraft, m.publishBundle, m.publishAsking, m.publishLog = nil, nil, "", nil
	m = m.openToolsOverlay(m.Theme.Tf("tools.publishPreparing", request.Name))
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	outcome := toolsOutcome{publishing: true}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		draft, err := use.Prepare(ctx, workspace, request)
		if err != nil {
			return err
		}
		outcome.draft = &draft
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// askPublishField asks the next missing catalog field in the composer, or
// builds the bundle when none is missing.
func (m AppModel) askPublishField() (AppModel, tea.Cmd) {
	draft := m.publishDraft
	if draft == nil {
		return m, nil
	}
	if len(draft.Missing) == 0 {
		m.publishAsking = ""
		return m.buildPublishBundle()
	}
	field := draft.Missing[0]
	m.publishAsking = field
	m.Composer.Input.Reset()
	if m.overlay == "tools" {
		m.Info.SetContent(publishDraftContent(*draft, m.Theme) + "\n" + m.Theme.Tf("tools.publishAsk", m.Theme.T(publishFieldLabels[field]), draft.Tool.Name))
		m.Info.Viewport.GotoTop()
	}
	m.Status.Error = ""
	m.Status.Notice = m.Theme.Tf("tools.publishAsk", m.Theme.T(publishFieldLabels[field]), draft.Tool.Name)
	return m, nil
}

// publishFieldKey takes a key while a catalog field is asked: Enter
// answers with the composer's text, Esc gives up, anything else types.
func (m AppModel) publishFieldKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch k.String() {
	case "enter":
		value := m.Composer.Input.Value()
		if err := application.SetPublishField(m.publishDraft, m.publishAsking, value); err != nil {
			m.Status.Error = err.Error()
			return m, nil, true
		}
		m.Composer.Input.Reset()
		next, cmd := m.askPublishField()
		return next, cmd, true
	case "esc", "ctrl+c":
		m.publishAsking, m.publishDraft = "", nil
		m.Composer.Input.Reset()
		m.Status.Notice = m.Theme.T("tools.publishKept")
		if m.overlay == "tools" {
			m.overlay = ""
		}
		return m, nil, true
	}
	return m, nil, false
}

// buildPublishBundle saves the catalog block and signs the bundle, then
// shows the publish card for its y.
func (m AppModel) buildPublishBundle() (AppModel, tea.Cmd) {
	draft := m.publishDraft
	if draft == nil {
		return m, nil
	}
	if m.Busy {
		m.Status.Notice = m.Theme.T("mode.busy")
		return m, nil
	}
	use := m.deps.ToolCatalog
	workspace := string(m.Header.State.Workspace)
	copied := *draft
	outcome := toolsOutcome{publishing: true}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		if err := use.SaveMetadata(ctx, workspace, copied); err != nil {
			return err
		}
		bundle, err := use.Bundle(ctx, copied)
		if err != nil {
			return err
		}
		outcome.bundle = &bundle
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// uploadPublishBundle sends the bundle after y; a 429 schedules a retry
// after its Retry-After.
func (m AppModel) uploadPublishBundle() (AppModel, tea.Cmd) {
	draft, bundle := m.publishDraft, m.publishBundle
	if draft == nil || bundle == nil {
		return m, nil
	}
	use := m.deps.ToolCatalog
	copied, signed := *draft, *bundle
	theme := m.Theme
	outcome := toolsOutcome{publishing: true}
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		report, err := use.Submit(ctx, copied, signed)
		if err != nil {
			var limited *application.RateLimited
			if errors.As(err, &limited) {
				outcome.retryAfter = limited.RetryAfter
				outcome.content = theme.Tf("tools.rateLimited", limited.RetryAfter.Round(time.Second))
				return nil
			}
			return err
		}
		outcome.report = &report
		return nil
	})
	return m, toolsCompletion(cmd, &outcome)
}

// completePublish applies a publish outcome: a draft to complete, a bundle
// to confirm, an upload's report, a retry to wait for.
func (m AppModel) completePublish(outcome toolsOutcome) (AppModel, tea.Cmd) {
	switch {
	case outcome.draft != nil:
		m.publishDraft = outcome.draft
		return m.askPublishField()
	case outcome.bundle != nil:
		if m.publishDraft == nil {
			return m, nil
		}
		m.publishBundle = outcome.bundle
		m.toolsAsking = true
		if m.overlay == "tools" {
			m.Info.SetContent(publishCard(*m.publishDraft, *outcome.bundle, m.Theme))
			m.Info.Viewport.GotoTop()
		}
		m.Status.Error = ""
		m.Status.Notice = m.Theme.Tf("tools.publishConfirm", m.publishDraft.Tool.Name, m.publishDraft.Metadata.Version, m.publishDraft.Catalog, outcome.bundle.KeyID)
		return m, nil
	case outcome.retryAfter > 0:
		m.publishLog = append(m.publishLog, outcome.content)
		m.renderPublishLog()
		m.Status.Notice = outcome.content
		gen := m.publishGen
		wait := outcome.retryAfter
		return m, tea.Tick(wait, func(time.Time) tea.Msg { return publishRetryMsg{gen: gen} })
	case outcome.report != nil:
		draft := m.publishDraft
		m.publishBundle = nil
		if draft == nil {
			// The panel was closed while the upload ran: the version is up
			// there all the same, so the status line says so.
			m.Status.Notice = m.Theme.Tf("tools.publishUploaded", outcome.report.Name, outcome.report.Version, "?")
			return m, nil
		}
		m.publishLog = []string{m.Theme.Tf("tools.publishUploaded", outcome.report.Name, outcome.report.Version, draft.Catalog), publishReport(*outcome.report, m.Theme)}
		if outcome.report.Settled() {
			return m.finishPublish(*outcome.report)
		}
		m.publishLog = append(m.publishLog, m.Theme.T("tools.publishPolling"))
		m.renderPublishLog()
		m.Status.Notice = m.Theme.Tf("tools.publishUploaded", outcome.report.Name, outcome.report.Version, draft.Catalog)
		m.publishDeadline = time.Now().Add(application.PublishPollDeadline)
		return m, m.pollPublish(publishPollInterval)
	}
	return m, nil
}

// pollPublish reads the version after delay, in the background: the
// console is not busy meanwhile.
func (m AppModel) pollPublish(delay time.Duration) tea.Cmd {
	draft := m.publishDraft
	if draft == nil {
		return nil
	}
	use := m.deps.ToolCatalog
	copied := *draft
	gen := m.publishGen
	ctx := m.lifetime.ctx
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return publishPollMsg{gen: gen, err: ctx.Err()}
		case <-time.After(delay):
		}
		ctx, cancel := context.WithTimeout(ctx, requestWait)
		defer cancel()
		report, err := use.Poll(ctx, copied)
		return publishPollMsg{gen: gen, report: report, err: err}
	}
}

const requestWait = time.Minute

// publishPollInterval is how long a poll waits; the tests shorten it.
var publishPollInterval = application.PublishPollInterval

// applyPublishPoll takes a poll's answer: shown, then polled again until
// the version settles or the deadline passes.
func (m AppModel) applyPublishPoll(msg publishPollMsg) (AppModel, tea.Cmd) {
	if msg.gen != m.publishGen || m.publishDraft == nil {
		return m, nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		var limited *application.RateLimited
		wait := publishPollInterval
		if errors.As(msg.err, &limited) {
			wait = limited.RetryAfter
			m.publishLog = append(m.publishLog, m.Theme.Tf("tools.rateLimited", wait.Round(time.Second)))
		} else {
			m.publishLog = append(m.publishLog, "▲ "+singleLine(msg.err.Error()))
		}
		if time.Now().Add(wait).After(m.publishDeadline) {
			return m.finishPublish(application.VersionReport{})
		}
		m.renderPublishLog()
		return m, m.pollPublish(wait)
	}
	m.publishLog = append(m.publishLog[:1], publishReport(msg.report, m.Theme))
	if msg.report.Settled() {
		return m.finishPublish(msg.report)
	}
	if time.Now().Add(publishPollInterval).After(m.publishDeadline) {
		return m.finishPublish(application.VersionReport{})
	}
	m.publishLog = append(m.publishLog, m.Theme.T("tools.publishPolling"))
	m.renderPublishLog()
	return m, m.pollPublish(publishPollInterval)
}

// finishPublish closes a publish run with its last report, or with the
// deadline passed when the report is empty.
func (m AppModel) finishPublish(report application.VersionReport) (AppModel, tea.Cmd) {
	m.publishDraft, m.publishBundle = nil, nil
	if report.Name == "" {
		m.publishLog = append(m.publishLog, m.Theme.T("tools.publishStillPending"))
		m.Status.Notice = m.Theme.T("tools.publishStillPending")
	} else {
		summary := m.Theme.Tf("tools.publishSettled", report.Name, report.Version, report.Status, testsWord(report, m.Theme))
		m.publishLog = append(m.publishLog, summary)
		m.Status.Notice = summary
	}
	m.renderPublishLog()
	return m, nil
}

// renderPublishLog shows the run's lines in the panel when it is open.
func (m *AppModel) renderPublishLog() {
	if m.overlay == "tools" {
		m.Info.SetContent(strings.Join(m.publishLog, "\n"))
		m.Info.Viewport.GotoBottom()
	}
}

// openToolsOverlay opens the panel with text.
func (m AppModel) openToolsOverlay(text string) AppModel {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(text)
	m.Info.Viewport.GotoTop()
	m.overlay = "tools"
	m.Status.Error = ""
	return m
}

// closePublish forgets a publish being asked or confirmed when the panel
// closes; a run that is polling goes on and reports in the status line.
func (m *AppModel) closePublish() {
	if m.publishAsking != "" || (m.publishBundle != nil && !m.Busy) {
		m.publishAsking, m.publishBundle, m.publishDraft = "", nil, nil
		m.Composer.Input.Reset()
		m.Status.Notice = m.Theme.T("tools.publishKept")
	}
}

// publishDraftContent is the draft as read: the tool, the catalog, the
// files and the fields known so far.
func publishDraftContent(draft application.PublishDraft, theme Theme) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", theme.Tf("tools.publishTitle", draft.Tool.Name, draft.Catalog, draft.API))
	fmt.Fprintf(&b, "  %s %s · %s %s · %s %s · %s %s\n", theme.T("tools.fieldVersion"), orDash(draft.Metadata.Version), theme.T("tools.fieldCategory"), orDash(draft.Metadata.Category), theme.T("tools.fieldLicense"), orDash(draft.Metadata.License), theme.T("tools.fieldTags"), orDash(strings.Join(draft.Metadata.Tags, ", ")))
	names := make([]string, 0, len(draft.Files))
	for name := range draft.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.files"), strings.Join(names, ", "))
	if draft.Tests != nil {
		fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.tests"), application.PublishTestsFile)
	}
	for _, warning := range draft.Warnings {
		fmt.Fprintf(&b, "▲ %s\n", singleLine(warning))
	}
	return b.String()
}

// publishCard is what the person reads before the upload: the draft, the
// bundle digest and key, then the manifest's forge card.
func publishCard(draft application.PublishDraft, bundle application.PublishBundle, theme Theme) string {
	var b strings.Builder
	b.WriteString(publishDraftContent(draft, theme))
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.bundleDigest"), bundle.Digest)
	key := bundle.KeyID
	if bundle.KeyCreated {
		key += " · " + theme.T("tools.keyCreatedNow")
	}
	fmt.Fprintf(&b, "  %s %s\n\n", theme.T("tools.keyID"), key)
	names := make([]string, 0, len(draft.Files))
	for name := range draft.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]application.ForgedFile, 0, len(names))
	for _, name := range names {
		files = append(files, application.ForgedFile{Path: name, Content: string(draft.Files[name])})
	}
	b.WriteString(forgeCardOf(draft.Tool.Program, draft.Tool.Args, draft.Tool.Description, draft.Tool.InputSchema, files))
	return b.String()
}

// publishReport renders a version report: the status, the findings and
// the tests.
func publishReport(report application.VersionReport, theme Theme) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.status"), report.Status)
	if report.RejectedReason != "" {
		fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.rejectedReason"), singleLine(report.RejectedReason))
	}
	if len(report.Findings) == 0 {
		fmt.Fprintf(&b, "  %s\n", theme.T("tools.noFindings"))
	} else {
		fmt.Fprintf(&b, "  %s\n", theme.T("tools.findingsTitle"))
		for _, finding := range report.Findings {
			where := finding.File
			if finding.Line > 0 {
				where = fmt.Sprintf("%s:%d", finding.File, finding.Line)
			}
			fmt.Fprintf(&b, "    %s · %s · %s · %s\n", finding.Severity, finding.Rule, where, singleLine(finding.Message))
		}
	}
	fmt.Fprintf(&b, "  %s %s\n", theme.T("tools.tests"), testsWord(report, theme))
	for _, failed := range report.FailedTests {
		fmt.Fprintf(&b, "    ✗ %s\n", singleLine(failed))
	}
	return strings.TrimRight(b.String(), "\n")
}

// testsWord says where the sandbox tests stand.
func testsWord(report application.VersionReport, theme Theme) string {
	switch {
	case report.TestsPassed == nil:
		return theme.T("tools.testsPending")
	case *report.TestsPassed:
		return theme.T("tools.testsPassed")
	default:
		return theme.T("tools.testsFailed")
	}
}
