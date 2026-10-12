package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const outcomeLine = "console: plan add-a-func-dce953 finished: done · 1/2 tasks done, 0 blocked, 1 skipped · sync green · changed: buildinfo/version.go, buildinfo/version_test.go · task sessions' cost unknown"

// planningModel is a console open on a planning session whose planner asked
// for approval, with a store and no model: a model request would fail.
func planningModel(t *testing.T, plans *fakePlans) (AppModel, *storage.SessionStore) {
	t.Helper()
	session, err := domain.NewSession(panelParent, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.BeginTurn("add buildinfo.Short() with a table test", nil); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "It's waiting for your approval on the plan card."}}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	m := update(New(Dependencies{Session: &session, Store: store, Start: application.StartTurnUseCase{Store: store}, Monochrome: true, Plans: plans}), tea.WindowSizeMsg{Width: 400, Height: 40})
	t.Cleanup(func() { m.zones.Close() })
	return m, store
}

func finishedPlan(status domain.PlanStatus) domain.PlanRecord {
	return domain.PlanRecord{ID: "add-a-func-dce953", Session: panelParent, Status: status, Updated: time.Now().Add(time.Minute),
		Tasks: []domain.PlanTask{
			{ID: "short", Status: domain.TaskDone, Handback: &domain.TaskHandback{Changed: []string{"buildinfo/version.go", "buildinfo/version_test.go"}}},
			{ID: "never-ran", Status: domain.TaskSkipped},
		},
		Syncs: []domain.SyncRecord{{Wave: 1, Verdict: "green"}}}
}

// On 10 October 2026 a plan synced green and its planning session still
// ended with the planner's "It's waiting for your approval on the plan
// card": its outcome is now a console line there, kept with the session and
// never sent to the model.
func TestAFinishedPlanShowsItsOutcomeInThePlanningSession(t *testing.T) {
	plans := &fakePlans{records: []domain.PlanRecord{finishedPlan(domain.PlanDone)}}
	m, store := planningModel(t, plans)
	next, cmd := m.Update(planEventMsg(application.PlanEvent{Plan: plans.records[0]}))
	m = drain(t, next.(AppModel), cmd)
	if m.Status.Error != "" {
		t.Fatalf("error: %s", m.Status.Error)
	}
	if text := m.Transcript.Text(); !strings.Contains(text, outcomeLine) || strings.Index(text, outcomeLine) < strings.Index(text, "waiting for your approval") {
		t.Fatalf("the transcript lacks the outcome after the planner's answer:\n%s", text)
	}
	if got := m.deps.Session.Messages(); len(got) != 2 {
		t.Fatalf("the outcome reached the model's history: %d messages", len(got))
	}
	// The same plan again adds nothing.
	next, cmd = m.Update(planEventMsg(application.PlanEvent{Plan: plans.records[0]}))
	m = drain(t, next.(AppModel), cmd)
	if n := strings.Count(m.Transcript.Text(), "finished: done"); n != 1 {
		t.Fatalf("the outcome is shown %d times", n)
	}

	loaded, err := store.Load(context.Background(), panelParent)
	if err != nil || len(loaded.ConsoleLines()) != 1 {
		t.Fatalf("the line was not saved: %v %+v", err, loaded.ConsoleLines())
	}
	reopened := update(New(Dependencies{Session: &loaded, Monochrome: true}), tea.WindowSizeMsg{Width: 400, Height: 40})
	defer reopened.zones.Close()
	if !strings.Contains(reopened.Transcript.Text(), outcomeLine) {
		t.Fatalf("the reopened session lacks the outcome:\n%s", reopened.Transcript.Text())
	}
}

// A plan that ends while a turn runs waits for the turn to end.
func TestAPlanEndingDuringATurnIsShownWhenTheTurnEnds(t *testing.T) {
	plans := &fakePlans{records: []domain.PlanRecord{finishedPlan(domain.PlanPartial)}}
	m, _ := planningModel(t, plans)
	m.Busy = true
	m.operationID = 7
	next, _ := m.Update(planEventMsg(application.PlanEvent{Plan: plans.records[0]}))
	m = next.(AppModel)
	if len(m.pendingPlanOutcomes) != 1 || strings.Contains(m.Transcript.Text(), "finished: partly done") {
		t.Fatalf("pending %d, transcript:\n%s", len(m.pendingPlanOutcomes), m.Transcript.Text())
	}
	next, cmd := m.Update(operationComplete{ID: 7, Session: *m.deps.Session})
	m = drain(t, next.(AppModel), cmd)
	if !strings.Contains(m.Transcript.Text(), "console: plan add-a-func-dce953 finished: partly done") || len(m.pendingPlanOutcomes) != 0 {
		t.Fatalf("the outcome was not recorded after the turn:\n%s", m.Transcript.Text())
	}
}

// A plan that ended while its session was closed is shown when it opens;
// one that ended before the person's last prompt, or another session's, is
// not.
func TestOpeningASessionShowsThePlansThatEndedWhileItWasClosed(t *testing.T) {
	old := finishedPlan(domain.PlanDone)
	old.ID, old.Updated = "older-plan-aaaaaa", time.Now().Add(-time.Hour)
	other := finishedPlan(domain.PlanDone)
	other.ID, other.Session = "other-plan-bbbbbb", "ffffffffffffffffffffffffffffffff"
	plans := &fakePlans{records: []domain.PlanRecord{old, other, finishedPlan(domain.PlanBlocked)}}
	m, _ := planningModel(t, plans)
	if len(m.pendingPlanOutcomes) != 1 || m.Init() == nil {
		t.Fatalf("pending %+v", m.pendingPlanOutcomes)
	}
	next, cmd := m.Update(planOutcomesMsg{})
	m = drain(t, next.(AppModel), cmd)
	text := m.Transcript.Text()
	if !strings.Contains(text, "console: plan add-a-func-dce953 finished: blocked") || strings.Contains(text, "older-plan") || strings.Contains(text, "other-plan") {
		t.Fatalf("transcript:\n%s", text)
	}
}

func TestPlanOutcomeLineInSpanishWithCostAndWaves(t *testing.T) {
	theme := Theme{Locale: Spanish, Monochrome: true}
	line := domain.ConsoleLine{AfterMessage: 2, Plan: &domain.PlanOutcome{PlanID: "p-123456", Status: domain.PlanPartial, Tasks: 5, Done: 3, Blocked: 1, Skipped: 1,
		Syncs: []domain.SyncVerdict{{Wave: 1, Verdict: "green"}, {Wave: 2, Verdict: "red"}}, Changed: []string{"a.go", "b.go", "c.go", "d.go", "e.go"}, CostUSD: 0.0123, CostKnown: true}}
	got := consoleLineRow(line, theme).Text
	want := "consola: el plan p-123456 terminó: hecho en parte · 3/5 tareas hechas · bloqueadas: 1 · omitidas: 1 · sync w1 green, w2 red · cambió: a.go, b.go, c.go +2 · las sesiones de tarea costaron " + FormatUSD(0.0123)
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// Switching to a session (Ctrl+O) shows the plans that ended while it was
// closed.
func TestSwitchingToASessionShowsThePlansThatEndedWhileItWasClosed(t *testing.T) {
	const otherID = "abcdefabcdefabcdefabcdefabcdefab"
	plan := finishedPlan(domain.PlanDone)
	plan.Session = otherID
	plans := &fakePlans{records: []domain.PlanRecord{plan}}
	m, _ := planningModel(t, plans)
	if len(m.pendingPlanOutcomes) != 0 {
		t.Fatalf("another session's plan was queued: %+v", m.pendingPlanOutcomes)
	}
	other, err := domain.NewSession(otherID, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.BeginTurn("plan something", nil); err != nil {
		t.Fatal(err)
	}
	if err := other.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "Approve the plan."}}); err != nil {
		t.Fatal(err)
	}
	m.Busy = true
	m.operationID = 7
	next, cmd := m.Update(operationComplete{ID: 7, Session: other})
	m = drain(t, next.(AppModel), cmd)
	if m.Header.State.ID != otherID || !strings.Contains(m.Transcript.Text(), "console: plan add-a-func-dce953 finished: done") {
		t.Fatalf("transcript of %s:\n%s", m.Header.State.ID, m.Transcript.Text())
	}
}
