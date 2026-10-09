package terminal

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// JobsPanelPort is the self-repair coordinator as /jobs shows it: every
// repair and improvement in the registry, the jobs the person starts and
// their pull requests. The console's coordinator implements it beside
// RepairPanelPort, so the panel finds it behind Dependencies.Repairs.
type JobsPanelPort interface {
	RepairPanelPort
	StartJob(ctx context.Context, parent domain.SessionState, kind, brief string) (domain.RepairRecord, error)
	// Live reports whether this console drives the job now.
	Live(id string) bool
	// ActiveLimit is jobs.max_active.
	ActiveLimit() int
	// PullRequestStatus reads the job's pull request from the forge.
	PullRequestStatus(ctx context.Context, id string) (application.PullRequestStatus, error)
	// QueueMerge queues the person's approval of the job's merge;
	// UnqueueMerge takes it back.
	QueueMerge(ctx context.Context, id string) error
	UnqueueMerge(ctx context.Context, id string) error
}

// jobsBriefBytes matches the brief StartJob accepts.
const jobsBriefBytes = 7 << 10

// JobsPanel lists every repair and improvement in the registry, whichever
// session or console started it, and starts new ones from a brief. Renders
// read the records the repairs panel caches and the readings g took; they
// never read the registry or the forge. Running eight jobs needed eight
// consoles on 9 October 2026; this panel is the one place to follow them.
type JobsPanel struct {
	// selectedID keeps the selection on its job when the list reloads.
	selectedID string
	details    bool
	// composing is the new-job form; improvement is its kind.
	composing, improvement bool
	brief                  textarea.Model
	// starting is true while the job's clone is prepared.
	starting bool
	// reasoning asks why the selected job's merge is declined.
	reasoning bool
	reason    textinput.Model
	// message is the panel's last word: a refused start or action.
	message string
	// readings are the pull request states g read, by job ID.
	readings map[string]string
}

// jobsMsg carries a background result into the program: a job that started
// or was refused, or a pull request reading.
type jobsMsg struct {
	started bool
	record  domain.RepairRecord
	id      string
	status  application.PullRequestStatus
	err     error
}

// jobsPort is the coordinator's jobs side, nil without MADE.
func (m AppModel) jobsPort() JobsPanelPort {
	jobs, _ := m.deps.Repairs.(JobsPanelPort)
	return jobs
}

// openJobsPanel reads the registry and shows every job.
func (m AppModel) openJobsPanel() AppModel {
	if m.jobsPort() == nil {
		m.Status.Error = m.Theme.T("jobs.unavailable")
		return m
	}
	m = m.loadRepairs()
	panel := &m.JobsPanel
	panel.details, panel.composing, panel.reasoning, panel.message = false, false, false, ""
	if panel.starting {
		panel.message = m.Theme.T("jobs.stillStarting")
	}
	m.overlay = "jobs"
	m.Status.Error = ""
	return m
}

// jobsSelected is the index of the selected job in the cached records.
func (m AppModel) jobsSelected() int {
	for i, record := range m.RepairPanel.Records {
		if record.ID == m.JobsPanel.selectedID {
			return i
		}
	}
	return 0
}

func (m AppModel) jobsRecord() (domain.RepairRecord, bool) {
	records := m.RepairPanel.Records
	if len(records) == 0 {
		return domain.RepairRecord{}, false
	}
	return records[m.jobsSelected()], true
}

func (m AppModel) jobsMove(delta int) AppModel {
	records := m.RepairPanel.Records
	if len(records) == 0 {
		return m
	}
	index := min(max(m.jobsSelected()+delta, 0), len(records)-1)
	m.JobsPanel.selectedID = records[index].ID
	return m
}

// jobsInput routes the panel's input; it leaves Ctrl+C and clicks to the
// console.
func (m AppModel) jobsInput(msg tea.Msg) (AppModel, tea.Cmd, bool) {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		if v.String() == "ctrl+c" {
			return m, nil, false
		}
		next, cmd := m.jobsKey(v)
		return next, cmd, true
	case tea.PasteMsg:
		var cmd tea.Cmd
		switch {
		case m.JobsPanel.composing:
			m.JobsPanel.brief, cmd = m.JobsPanel.brief.Update(v)
		case m.JobsPanel.reasoning:
			m.JobsPanel.reason, cmd = m.JobsPanel.reason.Update(v)
		}
		return m, cmd, true
	case tea.MouseWheelMsg:
		switch v.Button {
		case tea.MouseWheelUp:
			m = m.jobsMove(-1)
		case tea.MouseWheelDown:
			m = m.jobsMove(1)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m AppModel) jobsKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	panel := &m.JobsPanel
	if panel.composing {
		return m.jobsFormKey(k)
	}
	if panel.reasoning {
		switch k.String() {
		case "esc":
			panel.reasoning = false
			return m, nil
		case "enter":
			reason := strings.TrimSpace(panel.reason.Value())
			if reason == "" {
				panel.message = m.Theme.T("repairs.reasonRequired")
				return m, nil
			}
			panel.reasoning = false
			return m.jobsDecide(false, reason), nil
		}
		var cmd tea.Cmd
		panel.reason, cmd = panel.reason.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "esc":
		m.overlay = ""
		return m, nil
	case "up":
		return m.jobsMove(-1), nil
	case "down":
		return m.jobsMove(1), nil
	case "pgup":
		return m.jobsMove(-5), nil
	case "pgdown":
		return m.jobsMove(5), nil
	case "home":
		return m.jobsMove(-len(m.RepairPanel.Records)), nil
	case "end":
		return m.jobsMove(len(m.RepairPanel.Records)), nil
	case "enter":
		panel.details = !panel.details
		return m, nil
	case "n":
		if panel.starting {
			panel.message = m.Theme.T("jobs.stillStarting")
			return m, nil
		}
		return m.jobsForm(), nil
	}
	record, ok := m.jobsRecord()
	switch k.String() {
	case "a", "d", "r", "g", "m":
		if !ok {
			panel.message = m.Theme.T("jobs.nothingSelected")
			return m, nil
		}
	default:
		return m, nil
	}
	switch k.String() {
	case "a":
		if !record.Status.Awaiting() {
			panel.message = m.Theme.T("repairs.nothingToDecide")
			return m, nil
		}
		return m.jobsDecide(true, ""), nil
	case "d":
		switch record.Status {
		case domain.RepairAwaitingApproval:
			return m.jobsDecide(false, ""), nil
		case domain.RepairAwaitingMerge:
			input := textinput.New()
			input.Prompt = m.Theme.T("repairs.reasonPrompt")
			input.CharLimit = 2000
			input.SetWidth(max(1, m.Layout.Width-len(input.Prompt)-6))
			input.Focus()
			panel.reason, panel.reasoning = input, true
			return m, nil
		}
		panel.message = m.Theme.T("repairs.nothingToDecide")
		return m, nil
	case "r":
		if record.Status != domain.RepairInterrupted {
			panel.message = m.Theme.T("repairs.nothingToRecover")
			return m, nil
		}
		if err := m.jobsPort().Recover(m.lifetime.ctx, record.ID); err != nil {
			panel.message = err.Error()
			return m, nil
		}
		panel.message = ""
		return m.loadRepairs(), nil
	case "m":
		// m queues the merge, or takes a queued one back while it waits.
		var err error
		if record.Status == domain.RepairAwaitingMerge && !record.Queued.IsZero() {
			err = m.jobsPort().UnqueueMerge(m.lifetime.ctx, record.ID)
			panel.message = m.Theme.Tf("jobs.unqueued", record.ID)
		} else {
			err = m.jobsPort().QueueMerge(m.lifetime.ctx, record.ID)
			panel.message = m.Theme.Tf("jobs.queuedNow", record.ID)
		}
		if err != nil {
			panel.message = err.Error()
		}
		return m.loadRepairs(), nil
	}
	// g reads the pull request in the background; renders never do.
	if record.PullRequest == 0 {
		panel.message = m.Theme.T("jobs.noPullRequestToRead")
		return m, nil
	}
	port, ctx, id := m.jobsPort(), m.lifetime.ctx, record.ID
	panel.message = m.Theme.Tf("jobs.readingPullRequest", record.PullRequest)
	return m, func() tea.Msg {
		status, err := port.PullRequestStatus(ctx, id)
		return jobsMsg{id: id, status: status, err: err}
	}
}

// jobsDecide hands the person's decision to the selected job.
func (m AppModel) jobsDecide(approve bool, reason string) AppModel {
	record, ok := m.jobsRecord()
	if !ok {
		return m
	}
	if err := m.jobsPort().Decide(m.lifetime.ctx, record.ID, approve, reason); err != nil {
		m.JobsPanel.message = err.Error()
		return m
	}
	m.JobsPanel.message = ""
	return m.loadRepairs()
}

// jobsForm opens the new-job form: a kind and a multi-line brief.
func (m AppModel) jobsForm() AppModel {
	width, _ := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	brief := textarea.New()
	brief.ShowLineNumbers = false
	brief.Placeholder = m.Theme.T("jobs.briefPlaceholder")
	brief.Prompt = "  "
	brief.CharLimit = jobsBriefBytes
	brief.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))
	brief.SetWidth(max(10, width-4))
	brief.SetHeight(4)
	brief.Focus()
	m.JobsPanel.brief, m.JobsPanel.composing, m.JobsPanel.improvement, m.JobsPanel.message = brief, true, false, ""
	return m
}

func (m AppModel) jobsFormKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	panel := &m.JobsPanel
	switch k.String() {
	case "esc":
		panel.composing = false
		return m, nil
	case "tab":
		panel.improvement = !panel.improvement
		return m, nil
	case "enter":
		if panel.starting {
			return m, nil
		}
		brief := strings.TrimSpace(panel.brief.Value())
		if brief == "" {
			panel.message = m.Theme.T("jobs.briefRequired")
			return m, nil
		}
		kind := application.JobRepair
		if panel.improvement {
			kind = application.JobImprovement
		}
		port, ctx, parent := m.jobsPort(), m.lifetime.ctx, m.Header.State
		panel.starting, panel.message = true, m.Theme.Tf("jobs.starting", m.Theme.T("jobs.kind."+kind))
		return m, func() tea.Msg {
			record, err := port.StartJob(ctx, parent, kind, brief)
			return jobsMsg{started: true, record: record, err: err}
		}
	}
	var cmd tea.Cmd
	panel.brief, cmd = panel.brief.Update(k)
	return m, cmd
}

// jobsResult applies a background result: a refused start keeps the form
// and its brief, and shows why in the panel.
func (m AppModel) jobsResult(v jobsMsg) AppModel {
	panel := &m.JobsPanel
	if v.started {
		panel.starting = false
		m = m.loadRepairs()
		if v.err != nil {
			panel.message = m.Theme.Tf("jobs.refused", v.err.Error())
			if m.overlay != "jobs" {
				m.Status.Error = panel.message
			}
			return m
		}
		panel.composing, panel.selectedID = false, v.record.ID
		panel.message = m.Theme.Tf("jobs.started", m.Theme.T("jobs.kind."+v.record.Kind()), v.record.ID)
		return m
	}
	if panel.readings == nil {
		panel.readings = map[string]string{}
	}
	if v.err != nil {
		panel.readings[v.id] = m.Theme.Tf("jobs.readingFailed", v.err.Error())
	} else {
		panel.readings[v.id] = m.Theme.Tf("jobs.reading", orDash(v.status.State), orDash(v.status.MergeState), v.status.Passed, v.status.Pending, len(v.status.Failed))
		if len(v.status.Failed) > 0 {
			panel.readings[v.id] += " · " + strings.Join(v.status.Failed, "; ")
		}
	}
	panel.message = ""
	return m
}

// jobsCI is what the panel knows about a job's pull request checks: the
// last reading g took, or what the record's status implies.
func (m AppModel) jobsCI(record domain.RepairRecord) string {
	if reading, ok := m.JobsPanel.readings[record.ID]; ok {
		return reading
	}
	switch {
	case record.Status == domain.RepairCompleted:
		return m.Theme.T("jobs.ci.merged")
	case record.Status == domain.RepairAwaitingMerge:
		return m.Theme.T("jobs.ci.green")
	case record.Step == "watch" && record.Active():
		return m.Theme.T("jobs.ci.watching")
	}
	return m.Theme.T("jobs.ci.unknown")
}

// jobsQueuePlace is the job's place in the merge queue: first is merged
// first.
func (m AppModel) jobsQueuePlace(job domain.RepairRecord) int {
	place := 1
	for _, record := range m.RepairPanel.Records {
		if record.ID != job.ID && !record.Queued.IsZero() && record.Status == domain.RepairAwaitingMerge && (record.Queued.Before(job.Queued) || record.Queued.Equal(job.Queued) && record.ID < job.ID) {
			place++
		}
	}
	return place
}

// jobsOwner names the console that drives the job, when one should.
func (m AppModel) jobsOwner(record domain.RepairRecord) string {
	switch {
	case m.jobsPort().Live(record.ID):
		return m.Theme.T("jobs.owner.here")
	case record.Active():
		return m.Theme.T("jobs.owner.other")
	case record.Status == domain.RepairInterrupted:
		return m.Theme.T("jobs.owner.none")
	}
	return ""
}

// jobsBlock is one job's rows: a heading, its progress and, when selected
// with Enter, its details.
func (m AppModel) jobsBlock(record domain.RepairRecord, selected bool) []string {
	theme := m.Theme
	heading := theme.Tf(kindKey("repairs.row", record), record.ID, theme.T("repairs.status."+string(record.Status)))
	if owner := m.jobsOwner(record); owner != "" {
		heading += " · " + owner
	}
	if selected {
		heading = theme.Selected(heading)
	} else {
		heading = "  " + theme.Accent(heading)
	}
	step := theme.Tf("jobs.step", orDash(record.Step))
	if record.StepLimit > 0 {
		step = theme.Tf("jobs.stepBounded", record.Step, record.StepAttempt, record.StepLimit)
	}
	progress := step + " · " + theme.Tf("jobs.attempt", record.Attempt) + " · "
	if record.PullRequest > 0 {
		progress += theme.Tf("jobs.pullRequest", record.PullRequest, m.jobsCI(record))
	} else {
		progress += theme.T("jobs.noPullRequest")
	}
	lines := []string{heading, "    " + progress}
	if record.Pending != "" && record.Status.Awaiting() {
		lines = append(lines, "    "+theme.Tf("repairs.pending", record.Pending))
	}
	if record.Check != "" {
		lines = append(lines, "    "+theme.Tf("jobs.check", record.Check))
	}
	if !record.Queued.IsZero() {
		switch {
		case record.Status == domain.RepairInterrupted:
			lines = append(lines, "    "+theme.T("jobs.queuedStale"))
		case !record.Status.Terminal():
			lines = append(lines, "    "+theme.Tf("jobs.queued", m.jobsQueuePlace(record), record.Queued.Local().Format("15:04"), orDash(record.QueueNote)))
		}
	}
	if record.Error != "" {
		lines = append(lines, "    "+theme.Tf("repairs.error", record.Error))
	}
	if !selected || !m.JobsPanel.details {
		return lines
	}
	lines = append(lines, "    "+theme.T("jobs.details.brief"))
	for i, line := range strings.Split(strings.TrimSpace(record.Brief), "\n") {
		if i == 8 {
			lines = append(lines, "      …")
			break
		}
		lines = append(lines, "      "+line)
	}
	lines = append(lines, "    "+theme.Tf("jobs.details.origin", string(record.Parent), orDash(record.Build), record.Created.Local().Format("2006-01-02 15:04"), record.Updated.Local().Format("15:04")))
	if record.URL != "" {
		lines = append(lines, "    "+record.URL)
	}
	if record.MergeSHA != "" {
		lines = append(lines, "    "+theme.Tf("repairs.merged", shortDigest(record.MergeSHA)))
	}
	if record.Instance != "" {
		lines = append(lines, "    "+theme.Tf("repairs.instance", record.Instance))
	}
	if record.Memory != "" {
		lines = append(lines, "    "+theme.Tf("repairs.memory", record.Memory))
	}
	if record.Clone != "" {
		lines = append(lines, "    "+theme.Tf("repairs.clone", record.Clone, orDash(string(record.Session))))
	}
	return lines
}

// jobsPanelView draws the list, scrolled to the selected job, above the
// form, the reason input or the panel's last message.
func (m AppModel) jobsPanelView() string {
	width, height := m.Layout.Width, m.Layout.Height-1
	inner, bodyHeight := OverlayBodySize(width, height)
	panel := m.JobsPanel
	records := m.RepairPanel.Records
	active := 0
	for _, record := range records {
		if record.Active() {
			active++
		}
	}
	subtitle := m.Theme.T("jobs.hints")
	var tail []string
	switch {
	case panel.composing:
		subtitle = m.Theme.T("jobs.formHints")
		kind := application.JobRepair
		if panel.improvement {
			kind = application.JobImprovement
		}
		tail = append(tail, "", m.Theme.Accent(m.Theme.Tf("jobs.formKind", m.Theme.T("jobs.kind."+kind))))
		tail = append(tail, strings.Split(panel.brief.View(), "\n")...)
	case panel.reasoning:
		subtitle = m.Theme.T("repairs.reasonHints")
		tail = append(tail, "", panel.reason.View())
	}
	if panel.message != "" {
		tail = append(tail, "")
		tail = append(tail, strings.Split(ansi.Wrap(panel.message, max(10, inner-2), " "), "\n")...)
	}
	listHeight := max(1, bodyHeight-len(tail))
	var lines []string
	start, end := 0, 0
	selected := m.jobsSelected()
	for i, record := range records {
		if i > 0 {
			lines = append(lines, "")
		}
		if i == selected {
			start = len(lines)
		}
		lines = append(lines, m.jobsBlock(record, i == selected)...)
		if i == selected {
			end = len(lines)
		}
	}
	if len(records) == 0 {
		lines = []string{m.Theme.T("jobs.empty")}
	}
	offset := 0
	if end > listHeight {
		offset = min(start, end-listHeight)
	}
	lines = lines[offset:min(len(lines), offset+listHeight)]
	for len(lines) < listHeight {
		lines = append(lines, "")
	}
	body := strings.Join(append(lines, tail...), "\n")
	return m.Theme.Overlay(m.Theme.Tf("jobs.title", active, m.jobsPort().ActiveLimit()), subtitle, body, m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), width, height)
}
