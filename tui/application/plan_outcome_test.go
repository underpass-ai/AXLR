package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// plannedSession is a planning session after the person approved its plan:
// complete, its last answer the planner's.
func plannedSession(t *testing.T) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("add buildinfo.Short() with a table test", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("It's waiting for your approval on the plan card.")); err != nil {
		t.Fatal(err)
	}
	return s
}

func finishedPlanOf(s domain.Session, status domain.PlanStatus, updated time.Time) domain.PlanRecord {
	return domain.PlanRecord{ID: "add-a-func-dce953", Session: s.Export().ID, Status: status, Updated: updated,
		Tasks: []domain.PlanTask{
			{ID: "short", Status: domain.TaskDone, Session: "11111111111111111111111111111111", Handback: &domain.TaskHandback{Changed: []string{"buildinfo/version.go", "buildinfo/version_test.go"}}},
			{ID: "docs", Status: domain.TaskDone, Session: "22222222222222222222222222222222", Handback: &domain.TaskHandback{Changed: []string{"docs/console.md"}}},
			{ID: "never-ran", Status: domain.TaskSkipped},
		},
		Syncs: []domain.SyncRecord{{Wave: 1, Verdict: "green"}},
	}
}

// On 10 October 2026 a plan synced green and its planning session still
// ended with the planner's "waiting for your approval": a finished plan now
// leaves a console line there, without a model request.
func TestAFinishedPlanLeavesAConsoleLineInItsPlanningSession(t *testing.T) {
	s := plannedSession(t)
	messages := s.Messages()
	store := &memoryStore{}
	ledgers := usageLedgers{
		"11111111111111111111111111111111": {Cost: 0.005, CostRequests: 3},
		"22222222222222222222222222222222": {Cost: 0.003, CostRequests: 2},
	}
	u := StartTurnUseCase{Store: store, Continue: ContinueTurnUseCase{Usage: ledgers, Models: streamFunc(nil)}}
	plan := finishedPlanOf(s, domain.PlanDone, time.Now())
	events := 0
	added, err := u.RecordPlanOutcome(context.Background(), &s, plan, func(e Event) error {
		if e.Kind != EventSession {
			t.Fatalf("event %v", e.Kind)
		}
		events++
		return nil
	})
	if err != nil || !added || events != 1 || len(store.states) != 1 {
		t.Fatalf("added=%v err=%v events=%d saves=%d", added, err, events, len(store.states))
	}
	// No message: the model's history and the cache prefix are untouched.
	if !reflect.DeepEqual(s.Messages(), messages) || s.Status() != domain.StatusComplete {
		t.Fatal("recording a plan's outcome changed the model's history")
	}
	lines := s.ConsoleLines()
	if len(lines) != 1 || lines[0].AfterMessage != len(messages) {
		t.Fatalf("lines %+v", lines)
	}
	got := *lines[0].Plan
	if got.PlanID != plan.ID || got.Status != domain.PlanDone || got.Tasks != 3 || got.Done != 2 || got.Skipped != 1 ||
		!reflect.DeepEqual(got.Syncs, []domain.SyncVerdict{{Wave: 1, Verdict: "green"}}) ||
		!reflect.DeepEqual(got.Changed, []string{"buildinfo/version.go", "buildinfo/version_test.go", "docs/console.md"}) ||
		!got.CostKnown || got.CostUSD < 0.0079 || got.CostUSD > 0.0081 {
		t.Fatalf("outcome %+v", got)
	}
	if !reflect.DeepEqual(store.states[0].ConsoleLines, s.Export().ConsoleLines) {
		t.Fatal("the saved session lacks the line")
	}
	// Once per plan; another session's plan or an unfinished one adds nothing.
	other := plan
	other.Session = "ffffffffffffffffffffffffffffffff"
	running := plan
	running.ID, running.Status = "still-running", domain.PlanRunning
	for _, again := range []domain.PlanRecord{plan, other, running} {
		if added, err := u.RecordPlanOutcome(context.Background(), &s, again, ignoreEvent); added || err != nil {
			t.Fatalf("%s %s: added=%v err=%v", again.ID, again.Status, added, err)
		}
	}
	// Without ledgers the cost is unknown, not zero.
	bare := plannedSession(t)
	if _, err := (StartTurnUseCase{Store: store}).RecordPlanOutcome(context.Background(), &bare, plan, ignoreEvent); err != nil || bare.ConsoleLines()[0].Plan.CostKnown {
		t.Fatalf("without ledgers: %v %+v", err, bare.ConsoleLines())
	}
}

func TestUnreportedPlansAreThoseThatEndedAfterThePersonsLastPrompt(t *testing.T) {
	s := plannedSession(t)
	last := s.Export().MessageTimes[0]
	// A plan that ended while a turn ran, before its answer, is still due.
	state := s.Export()
	state.MessageTimes[1] = last.Add(10 * time.Minute)
	s, err := domain.RestoreSession(state)
	if err != nil {
		t.Fatal(err)
	}
	later := finishedPlanOf(s, domain.PlanPartial, last.Add(3*time.Minute))
	earliest := later
	earliest.ID, earliest.Updated = "first-plan", last.Add(time.Minute)
	before := later
	before.ID, before.Updated = "old-plan", last.Add(-time.Hour)
	declined := later
	declined.ID, declined.Status = "declined-plan", domain.PlanDeclined
	other := later
	other.ID, other.Session = "other-plan", "ffffffffffffffffffffffffffffffff"
	got := UnreportedPlans(s, []domain.PlanRecord{later, before, declined, other, earliest})
	if len(got) != 2 || got[0].ID != "first-plan" || got[1].ID != later.ID {
		t.Fatalf("unreported %+v", got)
	}
	if _, err := s.AddPlanOutcome(domain.NewPlanOutcome(later)); err != nil {
		t.Fatal(err)
	}
	if got := UnreportedPlans(s, []domain.PlanRecord{later}); len(got) != 0 {
		t.Fatal("a plan the session shows is still unreported")
	}
}
