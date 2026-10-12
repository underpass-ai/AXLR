package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type fakePlans struct {
	records []domain.PlanRecord
	started []string
	events  chan application.PlanEvent
}

func (f *fakePlans) Records(context.Context) ([]domain.PlanRecord, error) {
	return append([]domain.PlanRecord(nil), f.records...), nil
}
func (f *fakePlans) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return nil
}
func (f *fakePlans) Events() <-chan application.PlanEvent { return f.events }

func samplePlan(status domain.PlanStatus) domain.PlanRecord {
	return domain.PlanRecord{ID: "count-words-1a2b3c", Session: panelParent, Status: status, Waves: 2, Planner: "z-ai/glm-5.3-flash", Worker: "local/qwen",
		E2E: domain.CheckCommand{Program: "go", Args: []string{"test", "./..."}}, E2EBaseline: 1, Interfaces: "WordCount(s string) int",
		Tasks: []domain.PlanTask{
			{ID: "wordcount", Goal: "Fix WordCount", Wave: 1, Scope: []string{"textstat.go"}, UnitCheck: domain.CheckCommand{Program: "go", Args: []string{"test"}}, TestFirst: true, Status: domain.TaskDone,
				Handback: &domain.TaskHandback{Notes: []domain.TaskNote{{From: "wordcount", To: "all", Text: "uses Fields"}}, Questions: []string{"CRLF?"}}},
			{ID: "linecount", Goal: "Add LineCount", Wave: 2, DependsOn: []string{"wordcount"}, Scope: []string{"lines.go"}, UnitCheck: domain.CheckCommand{Program: "go", Args: []string{"test"}}, Status: domain.TaskRunning, Step: "green"},
		},
		Syncs:   []domain.SyncRecord{{Wave: 1, Verdict: "green"}},
		Updated: time.Now()}
}

func TestPlanPanelShowsPlansAndRunsAnInterruptedOneAgain(t *testing.T) {
	plans := &fakePlans{records: []domain.PlanRecord{samplePlan(domain.PlanRunning)}, events: make(chan application.PlanEvent, 1)}
	session, err := domain.NewSession(panelParent, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &session, Monochrome: true, Plans: plans}), tea.WindowSizeMsg{Width: 120, Height: 40})
	if badge := m.planBadge(); !strings.Contains(badge, "plan count-words-1a2b3c · wave 2/2 · linecount green") {
		t.Fatalf("badge = %q", badge)
	}
	m = m.openPlanPanel()
	view := m.View().Content
	for _, want := range []string{"Plans", "running", "wordcount", "uses Fields", "CRLF?", "sync wave 1 · green"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	plans.records[0].Status = domain.PlanInterrupted
	m = update(m, planEventMsg(application.PlanEvent{Plan: plans.records[0]}))
	m = update(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if len(plans.started) != 1 || plans.started[0] != "count-words-1a2b3c" {
		t.Fatalf("started %v", plans.started)
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.overlay != "" {
		t.Fatalf("n leaves the panel open: %q", m.overlay)
	}
}

func TestPlanCardShowsTheTaskTable(t *testing.T) {
	theme := New(Dependencies{Monochrome: true}).Theme
	run := domain.CeremonyRun{Plan: &domain.PlanRun{ID: "p", Returns: 1}}
	content := planCardContent(run, samplePlan(domain.PlanAwaitingApproval), nil, theme)
	for _, want := range []string{"Plan count-words-1a2b3c · 2 tasks in 2 waves · planned by z-ai/glm-5.3-flash · run by local/qwen", "go test ./... (baseline exit 1)", "Returns left: 1", "wave 1 · wordcount · Fix WordCount", "test first", "after wordcount", "WordCount(s string) int"} {
		if !strings.Contains(content, want) {
			t.Fatalf("card lacks %q:\n%s", want, content)
		}
	}
}

func planModel(t *testing.T, plans PlanPanelPort) AppModel {
	t.Helper()
	session, err := domain.NewSession(panelParent, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return update(New(Dependencies{Session: &session, Store: store, Monochrome: true, Plans: plans}), tea.WindowSizeMsg{Width: 120, Height: 40})
}

func runSlash(m AppModel, command string) AppModel {
	m.Composer.Input.SetValue(command)
	return update(m, ControlIntent("send"))
}

// On 10 October 2026, after a plan had run green, /plan set plan mode
// without a word, and the next prompt, a memory question, became a brief.
func TestPlanWithNoPlanRunningShowsThePlansAndArmsOnlyOnN(t *testing.T) {
	m := planModel(t, &fakePlans{})
	defer m.zones.Close()
	m = runSlash(m, "/plan")
	if m.overlay != "plans" || m.Header.State.Mode == domain.ModePlan {
		t.Fatalf("/plan with no plans set mode %q, overlay %q", m.Header.State.Mode, m.overlay)
	}
	if view := m.View().Content; !strings.Contains(view, "No plans yet") {
		t.Fatalf("the empty panel does not say how to start one:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.overlay != "" || m.Header.State.Mode != domain.ModePlan || m.Status.Notice != m.Theme.T("notice.planArmed") {
		t.Fatalf("n did not arm plan mode and say so: mode %q, notice %q", m.Header.State.Mode, m.Status.Notice)
	}
	if footer := m.footerView(); !strings.Contains(footer, "your next prompt is the brief") {
		t.Fatalf("the footer does not say the next prompt is the brief: %q", footer)
	}
}

func TestThePlansPanelShowsRecentPlansWithTheirOutcome(t *testing.T) {
	done := samplePlan(domain.PlanDone)
	done.ID, done.Brief = "add-a-func-dce953", "add buildinfo.Short() with a table test"
	done.Tasks = []domain.PlanTask{{ID: "buildinfo-short", Wave: 1, Status: domain.TaskDone, Handback: &domain.TaskHandback{Changed: []string{"buildinfo/version.go", "buildinfo/version_test.go"}}}}
	done.Syncs = []domain.SyncRecord{{Wave: 1, Verdict: "SYNCED", Rounds: 1}}
	older := samplePlan(domain.PlanDeclined)
	older.ID, older.Updated = "older-plan", done.Updated.Add(-time.Hour)
	m := planModel(t, &fakePlans{records: []domain.PlanRecord{older, done}})
	defer m.zones.Close()
	m = runSlash(m, "/plan")
	view := m.View().Content
	for _, want := range []string{"add-a-func-dce953 · done", "brief: add buildinfo.Short()", "changed buildinfo/version.go, buildinfo/version_test.go", "sync wave 1 · SYNCED", "older-plan · declined"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "add-a-func-dce953") > strings.Index(view, "older-plan") {
		t.Fatalf("the newest plan is not first:\n%s", view)
	}
	if m.Header.State.Mode == domain.ModePlan {
		t.Fatal("opening the panel armed plan mode")
	}
}

func TestPlanWithoutAPlanRunnerArmsPlanModeAndSaysSo(t *testing.T) {
	m := planModel(t, nil)
	defer m.zones.Close()
	m = runSlash(m, "/plan")
	if m.Header.State.Mode != domain.ModePlan || m.Status.Notice != m.Theme.T("notice.planArmed") {
		t.Fatalf("mode %q, notice %q", m.Header.State.Mode, m.Status.Notice)
	}
}

// docs/troubleshooting.md tells the person to find a task's session in the
// plans panel; on 12 October 2026 the panel never showed it.
func TestThePlansPanelShowsEachTasksSession(t *testing.T) {
	plan := samplePlan(domain.PlanRunning)
	plan.Tasks[0].Session = "89abcdef0123456789abcdef01234567"
	for _, locale := range []Locale{English, Spanish} {
		content := ansi.Strip(planPanelContent([]domain.PlanRecord{plan}, Theme{Locale: locale, Monochrome: true}, time.Now()))
		want := Translatef(locale, "plans.taskSession", "89abcdef")
		if !strings.Contains(content, want) || strings.Contains(content, "89abcdef0") {
			t.Fatalf("%s: the task's short session id is missing:\n%s", locale, content)
		}
	}
}
