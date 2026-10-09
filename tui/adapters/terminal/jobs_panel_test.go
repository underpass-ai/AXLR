package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// fakeJobs is the coordinator's jobs side over the repairs fake.
type fakeJobs struct {
	*fakeRepairs
	live      map[string]bool
	starts    []string
	startErr  error
	status    application.PullRequestStatus
	statusErr error
	reads     []string
	queue     []string
	queueErr  map[string]error
}

func (f *fakeJobs) StartJob(_ context.Context, parent domain.SessionState, kind, brief string) (domain.RepairRecord, error) {
	f.starts = append(f.starts, string(parent.ID)+":"+kind+":"+brief)
	if f.startErr != nil {
		return domain.RepairRecord{}, f.startErr
	}
	record := domain.RepairRecord{ID: "20261009-1000-new-job", Improvement: kind == application.JobImprovement, Signature: "n", Repository: "o/r", Parent: parent.ID, Status: domain.RepairPreparing, Attempt: 1, Created: time.Now()}
	f.records = append([]domain.RepairRecord{record}, f.records...)
	return record, nil
}
func (f *fakeJobs) QueueMerge(_ context.Context, id string) error {
	if err := f.queueErr[id]; err != nil {
		return err
	}
	f.queue = append(f.queue, "queue "+id)
	f.setQueued(id, time.Date(2026, 10, 9, 10, 5, 0, 0, time.Local))
	return nil
}
func (f *fakeJobs) UnqueueMerge(_ context.Context, id string) error {
	f.queue = append(f.queue, "unqueue "+id)
	f.setQueued(id, time.Time{})
	return nil
}
func (f *fakeJobs) setQueued(id string, at time.Time) {
	for i := range f.records {
		if f.records[i].ID == id {
			f.records[i].Queued, f.records[i].QueueNote = at, "queued"
		}
	}
}
func (f *fakeJobs) Live(id string) bool { return f.live[id] }
func (f *fakeJobs) ActiveLimit() int    { return 2 }
func (f *fakeJobs) PullRequestStatus(_ context.Context, id string) (application.PullRequestStatus, error) {
	f.reads = append(f.reads, id)
	return f.status, f.statusErr
}

// jobRecords are four jobs from three sessions and two consoles, newest
// first as the coordinator lists them.
func jobRecords() []domain.RepairRecord {
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	return []domain.RepairRecord{
		{ID: "20261009-0950-search-matches", Improvement: true, Signature: "a", Repository: "o/r", Parent: panelParent, Session: "s1", Status: domain.RepairAwaitingMerge, Step: "decide", PullRequest: 12, URL: "https://example.test/pr/12", Pending: "merge pull request #12: a approves and merges, d declines with a reason", Check: "pull request checks green: 14 passed", Attempt: 1, Brief: "local_search should list matches per file\n\nStarted by the person from /jobs", Created: now, Updated: now},
		{ID: "20261009-0940-edit-fails", Signature: "b", Repository: "o/r", Parent: "fedcba9876543210fedcba9876543210", Session: "s2", Status: domain.RepairRunning, Step: "repair", StepAttempt: 2, StepLimit: 3, Check: "go test ./... exited 1: FAIL", Attempt: 1, RunToken: "other", Created: now.Add(-10 * time.Minute), Updated: now},
		{ID: "20261009-0930-resize-loses-line", Signature: "c", Repository: "o/r", Parent: "fedcba9876543210fedcba9876543210", Session: "s3", Status: domain.RepairInterrupted, Step: "watch", PullRequest: 11, Error: "the console that drove it stopped", Attempt: 2, Created: now.Add(-20 * time.Minute), Updated: now},
		{ID: "20261009-0900-copy-selection", Improvement: true, Signature: "d", Repository: "o/r", Parent: "fedcba9876543210fedcba9876543210", Session: "s4", Status: domain.RepairCompleted, PullRequest: 10, MergeSHA: "abc123def4567890", Attempt: 1, Created: now.Add(-time.Hour), Updated: now},
	}
}

func jobsModel(t *testing.T, jobs *fakeJobs, locale Locale) AppModel {
	t.Helper()
	session, err := domain.NewSession(panelParent, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &session, Monochrome: true, Repairs: jobs, Locale: locale}), tea.WindowSizeMsg{Width: 140, Height: 44})
	if m.overlay == "repairs" {
		// A waiting merge opens the repairs panel on its own; close it.
		m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	}
	m.Composer.Input.SetValue("/jobs")
	return update(m, ControlIntent("send"))
}

func typeText(m AppModel, text string) AppModel {
	for _, r := range text {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// run applies a key and the background command it returns, as the program
// would.
func run(t *testing.T, m AppModel, msg tea.Msg) AppModel {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(AppModel)
	if cmd == nil {
		t.Fatal("no background command")
	}
	return update(m, cmd())
}

func TestJobsPanelListsEveryJobOfEveryConsole(t *testing.T) {
	jobs := &fakeJobs{fakeRepairs: &fakeRepairs{records: jobRecords()}, live: map[string]bool{"20261009-0950-search-matches": true}}
	m := jobsModel(t, jobs, English)
	if m.overlay != "jobs" {
		t.Fatalf("/jobs opened %q", m.overlay)
	}
	view := m.View().Content
	for _, want := range []string{
		"Jobs · 2 of 2 active",
		"> Improvement 20261009-0950-search-matches · waiting for your merge decision · this console",
		"step decide · attempt 1 · pull request #12 · checks green",
		"Decision: merge pull request #12",
		"Last check: pull request checks green: 14 passed",
		"Repair 20261009-0940-edit-fails · running · another console",
		"step repair 2/3 · attempt 1 · no pull request yet",
		"Repair 20261009-0930-resize-loses-line · interrupted · no console · r recovers it",
		"pull request #11 · g reads its checks",
		"Reason: the console that drove it stopped",
		"Improvement 20261009-0900-copy-selection · merged",
		"pull request #10 · merged",
		"n new",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Brief:") {
		t.Fatal("details are shown before Enter")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if view = m.View().Content; !strings.Contains(view, "Brief:") || !strings.Contains(view, "local_search should list matches per file") || !strings.Contains(view, "From session "+panelParent) {
		t.Fatalf("details:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "" {
		t.Fatal("esc closes the panel")
	}
	m = update(m, ControlIntent("palette"))
	m = update(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.overlay != "jobs" {
		t.Fatalf("the palette's j opens %q", m.overlay)
	}
}

func TestJobsPanelSpeaksSpanish(t *testing.T) {
	jobs := &fakeJobs{fakeRepairs: &fakeRepairs{records: jobRecords()}, live: map[string]bool{"20261009-0950-search-matches": true}}
	m := jobsModel(t, jobs, Spanish)
	view := m.View().Content
	for _, want := range []string{"Trabajos · 2 de 2 activos", "esta consola", "otra consola", "ninguna consola · r la recupera", "paso repair 2/3 · intento 1 · aún sin pull request", "g lee sus checks", "Último check: go test ./... exited 1: FAIL", "n nuevo"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	session, _ := domain.NewSession(panelParent, testWorkspace(), "model")
	alias := update(New(Dependencies{Session: &session, Monochrome: true, Repairs: jobs, Locale: Spanish}), tea.WindowSizeMsg{Width: 140, Height: 44})
	alias = update(alias, tea.KeyPressMsg{Code: tea.KeyEsc})
	alias.Composer.Input.SetValue("/trabajos")
	if alias = update(alias, ControlIntent("send")); alias.overlay != "jobs" {
		t.Fatalf("/trabajos opened %q", alias.overlay)
	}
}

func TestJobsPanelStartsAJobFromABriefAndShowsARefusal(t *testing.T) {
	jobs := &fakeJobs{fakeRepairs: &fakeRepairs{records: jobRecords()}, startErr: errors.New("2 of 2 jobs are active (jobs.max_active); wait for one to end or raise the limit")}
	m := jobsModel(t, jobs, English)
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !m.JobsPanel.composing || !strings.Contains(m.View().Content, "New repair (tab changes it)") {
		t.Fatalf("n opens the form:\n%s", m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.JobsPanel.message != Translate(English, "jobs.briefRequired") || len(jobs.starts) != 0 {
		t.Fatalf("empty brief: %q %v", m.JobsPanel.message, jobs.starts)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "search lists matches per file")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = typeText(m, "with line numbers")
	if !strings.Contains(m.View().Content, "New improvement") {
		t.Fatal("tab changes the kind")
	}
	m = run(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	want := panelParent + ":improvement:search lists matches per file\nwith line numbers"
	if len(jobs.starts) != 1 || jobs.starts[0] != want {
		t.Fatalf("starts %q, want %q", jobs.starts, want)
	}
	view := m.View().Content
	if !m.JobsPanel.composing || !strings.Contains(view, "Not started: 2 of 2 jobs are active (jobs.max_active)") {
		t.Fatalf("a refusal keeps the form and says why:\n%s", view)
	}
	jobs.startErr = nil
	m = run(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.JobsPanel.composing || m.JobsPanel.selectedID != "20261009-1000-new-job" || !strings.Contains(m.View().Content, "Started improvement 20261009-1000-new-job") {
		t.Fatalf("started:\n%s", m.View().Content)
	}
	if view := m.View().Content; !strings.Contains(view, "> Improvement 20261009-1000-new-job · preparing the clone") {
		t.Fatalf("the new job is listed and selected:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.JobsPanel.composing || m.overlay != "jobs" {
		t.Fatal("esc leaves the form, not the panel")
	}
}

func TestJobsPanelActsOnTheSelectedJob(t *testing.T) {
	records := jobRecords()
	records[1].Status, records[1].Pending = domain.RepairAwaitingApproval, "run the check command go test ./... in the clone"
	jobs := &fakeJobs{fakeRepairs: &fakeRepairs{records: records}, status: application.PullRequestStatus{State: "OPEN", MergeState: "BEHIND", Passed: 13, Pending: 1}}
	m := jobsModel(t, jobs, English)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if len(jobs.decisions) != 1 || jobs.decisions[0] != "20261009-0940-edit-fails:approve:" {
		t.Fatalf("a approves the selected job, not the first waiting one: %v", jobs.decisions)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = typeText(m, "not now")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(jobs.decisions) != 2 || jobs.decisions[1] != "20261009-0950-search-matches:deny:not now" {
		t.Fatalf("d declines the selected merge with a reason: %v", jobs.decisions)
	}
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if record, _ := m.jobsRecord(); record.ID != "20261009-0930-resize-loses-line" {
		t.Fatalf("the wheel selects: %s", record.ID)
	}
	m = update(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if len(jobs.recovered) != 1 || jobs.recovered[0] != "20261009-0930-resize-loses-line" {
		t.Fatalf("r recovers the selected job: %v", jobs.recovered)
	}
	if len(jobs.reads) != 0 {
		t.Fatal("renders read the forge")
	}
	m = run(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if len(jobs.reads) != 1 || !strings.Contains(m.View().Content, "pull request #11 · OPEN · BEHIND · 13 passed, 1 pending, 0 failed") {
		t.Fatalf("g reads the selected pull request %v:\n%s", jobs.reads, m.View().Content)
	}
	jobs.statusErr = errors.New("gh: not logged in")
	if m = run(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"}); !strings.Contains(m.View().Content, "checks unreadable: gh: not logged in") {
		t.Fatalf("a failed reading:\n%s", m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.JobsPanel.message != Translate(English, "jobs.noPullRequestToRead") {
		t.Fatalf("g without a pull request: %q", m.JobsPanel.message)
	}
	m = update(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.JobsPanel.message != Translate(English, "repairs.nothingToRecover") {
		t.Fatalf("r on a running job: %q", m.JobsPanel.message)
	}
	jobs.err = errors.New("repair x is not waiting for a decision")
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !strings.Contains(m.View().Content, "repair x is not waiting for a decision") {
		t.Fatalf("an action's error is shown in the panel:\n%s", m.View().Content)
	}
}

func TestJobsPanelQueuesTheSelectedMergeAndShowsItsPlace(t *testing.T) {
	jobs := &fakeJobs{fakeRepairs: &fakeRepairs{records: jobRecords()}, live: map[string]bool{"20261009-0950-search-matches": true},
		queueErr: map[string]error{"20261009-0940-edit-fails": errors.New("repair 20261009-0940-edit-fails is owned by another console; queue its merge there")}}
	m := jobsModel(t, jobs, English)
	m = update(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if len(jobs.queue) != 1 || jobs.queue[0] != "queue 20261009-0950-search-matches" {
		t.Fatalf("m queues the selected merge: %v", jobs.queue)
	}
	view := m.View().Content
	for _, want := range []string{"Merge queue #1 since 10:05 · queued", "Queued 20261009-0950-search-matches: it merges in turn", "m merge queue"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if len(jobs.queue) != 2 || jobs.queue[1] != "unqueue 20261009-0950-search-matches" || strings.Contains(m.View().Content, "Merge queue #1") {
		t.Fatalf("m takes a waiting merge back: %v", jobs.queue)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !strings.Contains(m.View().Content, "owned by another console") {
		t.Fatalf("a refusal is shown in the panel:\n%s", m.View().Content)
	}
	jobs.records[2].Queued = time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	m = update(m, repairEventMsg(application.RepairEvent{Record: jobs.records[2]}))
	if view := m.View().Content; !strings.Contains(view, "Queued for merge before its console stopped: r recovers it, then m queues it again") {
		t.Fatalf("an interrupted queued job:\n%s", view)
	}
}

func TestJobsPanelNeedsTheCoordinator(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("/jobs")
	m = update(m, ControlIntent("send"))
	if m.overlay != "" || m.Status.Error != Translate(English, "jobs.unavailable") {
		t.Fatalf("without MADE: %q %q", m.overlay, m.Status.Error)
	}
	empty := jobsModel(t, &fakeJobs{fakeRepairs: &fakeRepairs{}}, English)
	if !strings.Contains(empty.View().Content, "No repair or improvement yet") {
		t.Fatalf("empty panel:\n%s", empty.View().Content)
	}
	empty = update(empty, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if empty.JobsPanel.message != Translate(English, "jobs.nothingSelected") {
		t.Fatalf("a without jobs: %q", empty.JobsPanel.message)
	}
}
