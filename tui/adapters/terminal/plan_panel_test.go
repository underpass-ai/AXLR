package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
