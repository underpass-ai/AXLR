package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type syncEngine struct {
	fakeEngine
}

var syncTransitions = map[string]string{"integrated": "SYNCED", "conflict": "RECONCILE", "exhausted": "BLOCKED", "reconciled": "INTEGRATE"}

func (e *syncEngine) Transition(_ context.Context, _, trigger string) (string, error) {
	e.calls = append(e.calls, "transition "+trigger)
	return syncTransitions[trigger], nil
}

func TestASyncReconcilesTwiceThenBlocks(t *testing.T) {
	for name, tc := range map[string]struct {
		exits   []int
		verdict string
		rounds  int
	}{
		"green at once":       {[]int{0}, "green", 0},
		"green after a round": {[]int{1, 0}, "green", 1},
		"two red rounds":      {[]int{1, 1, 1}, "blocked", 2},
	} {
		t.Run(name, func(t *testing.T) {
			engine := &syncEngine{}
			d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: tc.exits}, Now: func() time.Time { return time.Unix(1, 0) }}
			plan := domain.PlanRecord{ID: "p", Session: "s", E2E: domain.CheckCommand{Program: "go", Args: []string{"test"}}, Tasks: []domain.PlanTask{{ID: "a", Wave: 1, Scope: []string{"a.go"}}, {ID: "b", Wave: 1, Scope: []string{"b.go"}}}}
			calls := 0
			outcome, err := d.RunSync(context.Background(), plan, 1, func(_ context.Context, round int, affected []domain.PlanTask, failing string) ([]SyncResponse, error) {
				calls++
				if round != calls || len(affected) != 2 {
					t.Fatalf("round %d affected %d", round, len(affected))
				}
				return []SyncResponse{{Task: "a", Summary: "tried"}}, nil
			})
			if err != nil || outcome.Verdict != tc.verdict || outcome.Rounds != tc.rounds || calls != tc.rounds {
				t.Fatalf("outcome %+v calls=%d err=%v; engine %v", outcome, calls, err, engine.calls)
			}
		})
	}
}

func TestAffectedTasksAreThoseTheFailureNames(t *testing.T) {
	tasks := []domain.PlanTask{{ID: "a", Scope: []string{"pkg/a.go"}}, {ID: "b", Scope: []string{"pkg/b.go"}}}
	if got := affectedTasks(tasks, "FAIL pkg/b.go:12: want 2"); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("affected = %+v", got)
	}
	if got := affectedTasks(tasks, "build failed"); len(got) != 2 {
		t.Fatalf("unnamed failure must involve the whole wave: %+v", got)
	}
}

// scriptedBench plays a worker: each task ends with the scripted status.
type scriptedBench struct {
	files   map[string][]byte
	plans   *memoryPlans
	outcome map[string]domain.TaskStatus
	order   []string
	syncs   []int
}

func (b *scriptedBench) BeginTask(_ context.Context, s *domain.Session, _ domain.PlanRecord, task domain.PlanTask) error {
	b.order = append(b.order, task.ID)
	return s.SetCeremony(domain.CeremonyRun{Definition: "axlr_task", Version: "1.0", Instance: "i-" + task.ID, Step: "green", Iteration: 1, Task: &domain.TaskRun{Plan: "p", Task: task.ID}})
}
func (b *scriptedBench) Begin(_ context.Context, s *domain.Session, _ root.Text, _ func(Event) error) error {
	run, _ := s.Ceremony()
	if b.files != nil {
		b.files["shared.go"] = []byte("half done by " + run.Task.Task)
	}
	record := b.plans.records[0]
	t, _ := record.Task(run.Task.Task)
	t.Status = b.outcome[t.ID]
	_ = b.plans.Save(context.Background(), record)
	s.FinishCeremony()
	return nil
}
func (b *scriptedBench) RunSync(_ context.Context, _ domain.PlanRecord, wave int, _ Reconciler) (SyncOutcome, error) {
	b.syncs = append(b.syncs, wave)
	return SyncOutcome{Instance: fmt.Sprint("sync", wave), Verdict: "green"}, nil
}
func (b *scriptedBench) Digest(_ context.Context, path string) string {
	if content, ok := b.files[path]; ok {
		return string(content)
	}
	return ""
}
func (b *scriptedBench) ReadFile(_ context.Context, path string) ([]byte, bool, error) {
	content, ok := b.files[path]
	return content, ok, nil
}
func (b *scriptedBench) WriteFile(_ context.Context, path string, content []byte) error {
	b.files[path] = content
	return nil
}
func (b *scriptedBench) Resolve(context.Context, *domain.Session, root.ToolCallID, domain.ToolDecision, func(Event) error) error {
	return errors.New("unused")
}
func (b *scriptedBench) Decide(context.Context, *domain.Session, bool, string, func(Event) error) error {
	return errors.New("unused")
}
func (b *scriptedBench) Continue(context.Context, *domain.Session, func(Event) error) error {
	return errors.New("unused")
}
func (b *scriptedBench) Observe(CeremonyObserverPort) {}
func (b *scriptedBench) Close() error                 { return nil }

type benchPort struct{ bench *scriptedBench }

func (p benchPort) Open(context.Context, string) (PlanWorkbench, error) { return p.bench, nil }

func TestARunnerRunsWavesInOrderAndSkipsWhatDependsOnABlockedTask(t *testing.T) {
	plans := &memoryPlans{}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", Status: domain.PlanReady, Waves: 2, Tasks: []domain.PlanTask{
		{ID: "a", Wave: 1, Status: domain.TaskPending},
		{ID: "b", Wave: 1, Status: domain.TaskPending},
		{ID: "c", Wave: 2, DependsOn: []string{"a"}, Status: domain.TaskPending},
		{ID: "d", Wave: 2, DependsOn: []string{"b"}, Status: domain.TaskPending},
	}})
	bench := &scriptedBench{plans: plans, outcome: map[string]domain.TaskStatus{"a": domain.TaskDone, "b": domain.TaskBlocked, "c": domain.TaskDone}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}, RunToken: "t"}
	if err := runner.Start(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	runner.Close()
	record := plans.records[0]
	statuses := []string{}
	for _, task := range record.Tasks {
		statuses = append(statuses, task.ID+"="+string(task.Status))
	}
	if strings.Join(bench.order, ",") != "a,b,c" || strings.Join(statuses, ",") != "a=done,b=blocked,c=done,d=skipped" {
		t.Fatalf("order %v statuses %v", bench.order, statuses)
	}
	// Wave 1 had a blocked task, so it is not integrated; wave 2 is not
	// complete either (d was skipped).
	if len(bench.syncs) != 0 || record.Status != domain.PlanPartial {
		t.Fatalf("syncs %v status %s", bench.syncs, record.Status)
	}
}

func TestARunnerSyncsEachFinishedWaveAndEndsDone(t *testing.T) {
	plans := &memoryPlans{}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", Status: domain.PlanReady, Waves: 2, Tasks: []domain.PlanTask{
		{ID: "a", Wave: 1, Status: domain.TaskPending}, {ID: "b", Wave: 2, DependsOn: []string{"a"}, Status: domain.TaskPending},
	}})
	bench := &scriptedBench{plans: plans, outcome: map[string]domain.TaskStatus{"a": domain.TaskDone, "b": domain.TaskDone}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}, RunToken: "t"}
	if err := runner.Start(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	runner.Close()
	if record := plans.records[0]; record.Status != domain.PlanDone || len(record.Syncs) != 2 || fmt.Sprint(bench.syncs) != "[1 2]" {
		t.Fatalf("record %+v syncs %v", record, bench.syncs)
	}
}

func TestABlockedTaskPutsItsScopeBack(t *testing.T) {
	plans := &memoryPlans{}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", Status: domain.PlanReady, Waves: 2, Tasks: []domain.PlanTask{
		{ID: "a", Wave: 1, Scope: []string{"shared.go"}, Status: domain.TaskPending},
		{ID: "b", Wave: 2, Scope: []string{"shared.go"}, DependsOn: []string{"a"}, Status: domain.TaskPending},
	}})
	bench := &scriptedBench{files: map[string][]byte{"shared.go": []byte("original")}, plans: plans, outcome: map[string]domain.TaskStatus{"a": domain.TaskDone, "b": domain.TaskBlocked}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}, RunToken: "t"}
	if err := runner.Start(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	runner.Close()
	// a's work stays; b's half-done change is undone back to a's.
	if got := string(bench.files["shared.go"]); got != "half done by a" {
		t.Fatalf("shared.go = %q", got)
	}
	if reason := plans.records[0].Tasks[1].Reason; !strings.Contains(reason, "restored shared.go") {
		t.Fatalf("reason = %q", reason)
	}
}
