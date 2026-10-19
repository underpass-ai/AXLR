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

type benchPort struct{ bench PlanWorkbench }

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

// binaryBench holds one binary scope file the runtime cannot read as text.
type binaryBench struct{ scriptedBench }

func (b *binaryBench) ReadFile(ctx context.Context, path string) ([]byte, bool, error) {
	if path == "logo.png" {
		return nil, false, fmt.Errorf("read %s: binary data: %w", path, ErrNotText)
	}
	return b.scriptedBench.ReadFile(ctx, path)
}
func (b *binaryBench) Digest(ctx context.Context, path string) string {
	if path == "logo.png" {
		return opaqueDigest
	}
	return b.scriptedBench.Digest(ctx, path)
}

// A binary scope file that was there before the task is not one it left
// behind, though it cannot be read back to restore it (20 October 2026).
func TestABlockedTaskDoesNotNameABinaryScopeFileAsNew(t *testing.T) {
	plans := &memoryPlans{}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", Status: domain.PlanReady, Waves: 1, Tasks: []domain.PlanTask{
		{ID: "a", Wave: 1, Scope: []string{"shared.go", "logo.png"}, Status: domain.TaskPending},
	}})
	bench := &binaryBench{scriptedBench{files: map[string][]byte{"shared.go": []byte("original")}, plans: plans, outcome: map[string]domain.TaskStatus{"a": domain.TaskBlocked}}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}, RunToken: "t"}
	if err := runner.Start(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	runner.Close()
	if reason := plans.records[0].Tasks[0].Reason; !strings.Contains(reason, "restored shared.go") || strings.Contains(reason, "logo.png") {
		t.Fatalf("reason = %q", reason)
	}
}

// reconcileBench is a scripted bench whose Begin plays a reconciling worker:
// it changes its scope file and ends with a summary and a note.
type reconcileBench struct {
	scriptedBench
}

func (b *reconcileBench) Begin(_ context.Context, s *domain.Session, prompt root.Text, _ func(Event) error) error {
	b.files["a.go"] = []byte("fixed")
	if err := s.BeginTurn(prompt, nil); err != nil {
		return err
	}
	return s.CompleteAssistant(assistant("Aligned a.go with b.\nNOTE b: please keep the signature"))
}

func TestReconcileWorkersReportChangesAndNotes(t *testing.T) {
	plans := &memoryPlans{}
	plan := domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", E2E: domain.CheckCommand{Program: "go", Args: []string{"test"}}, Tasks: []domain.PlanTask{
		{ID: "a", Goal: "A", Wave: 1, Scope: []string{"a.go"}, Handback: &domain.TaskHandback{Summary: "did a", Changed: []string{"a.go"}}},
		{ID: "b", Goal: "B", Wave: 1, Scope: []string{"b.go"}, Handback: &domain.TaskHandback{Summary: "did b", Notes: []domain.TaskNote{{From: "b", To: "a", Text: "renamed Foo"}}}},
	}}
	_ = plans.Save(context.Background(), plan)
	bench := &reconcileBench{scriptedBench{files: map[string][]byte{"a.go": []byte("old")}, plans: plans}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}}
	responses, err := runner.reconciler(context.Background(), bench, "p")(context.Background(), 1, plan.Tasks[:1], "FAIL a.go:3")
	if err != nil || len(responses) != 1 {
		t.Fatalf("responses %+v %v", responses, err)
	}
	r := responses[0]
	if r.Task != "a" || strings.Join(r.Changed, ",") != "a.go" || len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "keep the signature") {
		t.Fatalf("response %+v", r)
	}
	packet := SyncPacket(plan, plan.Tasks[0], 1, "FAIL a.go:3")
	for _, want := range []string{"round 1 of 2", "Your hand-back: did a", "Task b: did b", "note from b: renamed Foo", "FAIL a.go:3", "NOTE <task id or all>"} {
		if !strings.Contains(packet, want) {
			t.Errorf("packet misses %q", want)
		}
	}
}

func TestRunnerReconcileMarksForeignRunsInterruptedAndLists(t *testing.T) {
	plans := &memoryPlans{}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "old", Session: "s", Status: domain.PlanRunning, RunToken: "previous", Updated: time.Unix(1, 0), Tasks: []domain.PlanTask{{ID: "t", Status: domain.TaskRunning}}})
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "mine", Session: "s", Status: domain.PlanRunning, RunToken: "now", Updated: time.Unix(2, 0)})
	runner := &PlanRunner{Plans: plans, RunToken: "now"}
	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	records, err := runner.Records(context.Background())
	if err != nil || len(records) != 2 || records[0].ID != "old" {
		t.Fatalf("records (newest first) %+v %v", records, err)
	}
	old, mine := records[0], records[1]
	if old.Status != domain.PlanInterrupted || old.Tasks[0].Status != domain.TaskPending || mine.Status != domain.PlanRunning {
		t.Fatalf("reconcile: %+v", records)
	}
	if runner.Events() == nil {
		t.Fatal("no event channel")
	}
	if err := runner.Start(context.Background(), "mine"); err == nil {
		t.Fatal("a running plan started twice")
	}
	runner.Close()
}

func TestPlanTaskAndSyncOutcomesReachMemory(t *testing.T) {
	memory := &fakeMemory{}
	d, _, plans, _, s := planDriver(t)
	d.Memory = memory
	step(t, d, &s, planArgs(t, goodTasks))
	if _, err := d.PlanProposal(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Approve(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if len(memory.records) == 0 || memory.records[len(memory.records)-1].Kind != "decision" || memory.labels[len(memory.labels)-1]["plan"][0] != plans.records[0].ID {
		t.Fatalf("plan memory: %+v %+v", memory.records, memory.labels)
	}
	outcome := d.recordSync(context.Background(), plans.records[0], 1, SyncOutcome{Instance: "i", Verdict: "red", Output: "FAIL"}, "ws:x", []SyncResponse{{Task: "a", Changed: []string{"a.go"}, Summary: "s", Notes: []string{"b: x"}}})
	last := memory.records[len(memory.records)-1]
	if !strings.HasPrefix(outcome, "recorded") || !strings.Contains(last.Evidence, "note: b: x") || !strings.Contains(last.Evidence, "failing tail") {
		t.Fatalf("sync memory: %s %+v", outcome, last)
	}
}

// liveRuns is the launches that still hold their run tokens.
type liveRuns map[string]bool

func (l liveRuns) Alive(token string) bool { return l[token] }

// A plan another open console is running is left to it: a second launch
// neither marks it interrupted nor starts it again. On 12 October 2026 it
// did both, and the plan that went on running ended "done" with "the
// console that ran it stopped".
func TestReconcileLeavesThePlansOfAnOpenConsole(t *testing.T) {
	ctx := context.Background()
	plans := &memoryPlans{}
	_ = plans.Save(ctx, domain.PlanRecord{ID: "theirs", Session: "s", Status: domain.PlanRunning, RunToken: "console-a", Tasks: []domain.PlanTask{{ID: "t", Status: domain.TaskRunning}}})
	_ = plans.Save(ctx, domain.PlanRecord{ID: "stopped", Session: "s", Status: domain.PlanRunning, RunToken: "console-gone"})
	_ = plans.Save(ctx, domain.PlanRecord{ID: "paused", Session: "s", Status: domain.PlanInterrupted, RunToken: "console-a"})
	runner := &PlanRunner{Plans: plans, RunToken: "console-b", Liveness: liveRuns{"console-a": true}}
	defer runner.Close()
	if err := runner.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	theirs, _ := runner.load(ctx, "theirs")
	stopped, _ := runner.load(ctx, "stopped")
	if theirs.Status != domain.PlanRunning || theirs.Error != "" || theirs.Tasks[0].Status != domain.TaskRunning || stopped.Status != domain.PlanInterrupted {
		t.Fatalf("reconciled %+v %+v", theirs, stopped)
	}
	if err := runner.Start(ctx, "paused"); err == nil || !strings.Contains(err.Error(), "another console") {
		t.Fatalf("started the interrupted plan of an open console: %v", err)
	}
}

// A plan that reaches its end drops an error written while it ran, such as
// the one another console's reconcile wrote before this change.
func TestAFinishedPlanClearsAStaleError(t *testing.T) {
	ctx := context.Background()
	plans := &memoryPlans{}
	_ = plans.Save(ctx, domain.PlanRecord{ID: "p", Session: "s", Workspace: domain.Workspace(t.TempDir()), Worker: "local/gemma", Status: domain.PlanReady, Waves: 1, Tasks: []domain.PlanTask{{ID: "a", Wave: 1, Status: domain.TaskPending}}})
	bench := &staleErrorBench{scriptedBench{plans: plans, outcome: map[string]domain.TaskStatus{"a": domain.TaskDone}}}
	runner := &PlanRunner{Plans: plans, Store: &memoryStore{}, Workbench: benchPort{bench}, RunToken: "now"}
	if err := runner.Start(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	runner.Wait()
	record, _ := runner.load(ctx, "p")
	if record.Status != domain.PlanDone || record.Error != "" {
		t.Fatalf("finished plan %+v", record)
	}
	runner.Close()
}

// staleErrorBench writes an error on the plan while its task runs.
type staleErrorBench struct{ scriptedBench }

func (b *staleErrorBench) Begin(ctx context.Context, s *domain.Session, prompt root.Text, emit func(Event) error) error {
	b.plans.records[0].Error = "the console that ran it stopped; start it again from /plan"
	return b.scriptedBench.Begin(ctx, s, prompt, emit)
}
