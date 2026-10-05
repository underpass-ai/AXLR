package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const validRepairRequest = `{"description":"local_edit fails with internal_error on every replace of a.go","expected":"the edit is applied and the digest returned","observed":"status failed, internal_error: unexpected nil in replace, twice","evidence":["call c1 failed with internal_error","call c2 failed identically after a retry"],"tool_calls":["c1","c2"]}`

type fakeRegistry struct {
	mu      sync.Mutex
	records []domain.RepairRecord
	fail    error
}

func (f *fakeRegistry) Load(context.Context) ([]domain.RepairRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	return append([]domain.RepairRecord(nil), f.records...), nil
}

func (f *fakeRegistry) Save(_ context.Context, record domain.RepairRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.records {
		if f.records[i].ID == record.ID {
			f.records[i] = record
			return nil
		}
	}
	f.records = append(f.records, record)
	return nil
}

func (f *fakeRegistry) find(id string) (domain.RepairRecord, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, record := range f.records {
		if record.ID == id {
			return record, true
		}
	}
	return domain.RepairRecord{}, false
}

type fakeClones struct {
	mu       sync.Mutex
	dir      string
	err      error
	requests []RepairCloneRequest
}

func (f *fakeClones) Prepare(_ context.Context, request RepairCloneRequest) (RepairClone, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	if f.err != nil {
		return RepairClone{}, f.err
	}
	path := filepath.Join(f.dir, request.Slug)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return RepairClone{}, err
	}
	return RepairClone{Path: path, Base: "main"}, nil
}

// repairStore keeps sessions by ID so a recovery can load the repair session.
type repairStore struct {
	mu       sync.Mutex
	sessions map[domain.SessionID]domain.Session
}

func (s *repairStore) Save(_ context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[domain.SessionID]domain.Session{}
	}
	s.sessions[session.Export().ID] = session
	return nil
}
func (s *repairStore) Load(_ context.Context, id domain.SessionID) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return domain.Session{}, errors.New("unknown session")
	}
	return session, nil
}
func (s *repairStore) List(context.Context) ([]domain.SessionSummary, error) { return nil, nil }

// workbenchCalls is what a scenario fake recorded, copied out of its lock.
type workbenchCalls struct {
	begins, continues, resolves, decides, closed int
	prompts, decisions                           []string
	resolvedAs                                   []domain.ToolDecision
}

// workbenchBase counts calls and keeps the observer; the scenario fakes embed it.
type workbenchBase struct {
	mu       sync.Mutex
	observer CeremonyObserverPort
	workbenchCalls
}

func (w *workbenchBase) Observe(observer CeremonyObserverPort) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.observer = observer
}
func (w *workbenchBase) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed++
	return nil
}
func (w *workbenchBase) count(field *int, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	*field++
	if text != "" {
		w.prompts = append(w.prompts, text)
	}
}
func (w *workbenchBase) observe(progress CeremonyProgress) {
	w.mu.Lock()
	observer := w.observer
	w.mu.Unlock()
	if observer != nil {
		observer.Observe(progress)
	}
}
func (w *workbenchBase) snapshot() workbenchCalls {
	w.mu.Lock()
	defer w.mu.Unlock()
	return workbenchCalls{begins: w.begins, continues: w.continues, resolves: w.resolves, decides: w.decides, closed: w.closed, prompts: append([]string(nil), w.prompts...), decisions: append([]string(nil), w.decisions...), resolvedAs: append([]domain.ToolDecision(nil), w.resolvedAs...)}
}

func repairRunFor(step string, repair *domain.RepairRun) domain.CeremonyRun {
	return domain.CeremonyRun{Definition: "axlr_repair", Version: "1.0", Instance: "axlr-child-1", Step: step, Iteration: 1, Fence: "f", Repair: repair}
}

// happyWorkbench: the model proposes the check command, the person approves
// it, the console carries the ceremony to an automatic merge.
type happyWorkbench struct{ workbenchBase }

func (w *happyWorkbench) Begin(_ context.Context, s *domain.Session, prompt root.Text, _ func(Event) error) error {
	w.count(&w.begins, string(prompt))
	if err := s.BeginTurn(prompt, turnTools()); err != nil {
		return err
	}
	if err := s.SetCeremony(repairRunFor("reproduce", &domain.RepairRun{Repository: "o/r", Branch: "repair/x", Slug: "x"})); err != nil {
		return err
	}
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "reproduce"})
	call := root.ToolCall{ID: root.ToolCallID(fmt.Sprintf("step-%d", w.snapshot().begins)), Name: HostStepDoneName, Arguments: mustJSON(`{"check_command":{"program":"go","args":["test","./..."]},"expected":"pass","observed":"fail"}`)}
	return s.CompleteAssistant(assistant("", call))
}
func (w *happyWorkbench) Resolve(_ context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, _ func(Event) error) error {
	w.count(&w.resolves, "")
	w.mu.Lock()
	w.resolvedAs = append(w.resolvedAs, decision)
	w.mu.Unlock()
	if err := s.RecordToolOutcome(id, decision, domain.ToolOutcome{Content: `{"accepted":true}`}); err != nil {
		return err
	}
	if decision == domain.DecisionDeny {
		// The model declares the failure not reproducible and the console
		// closes the ceremony BLOCKED with the reason.
		w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "reproduce", State: "BLOCKED", Terminal: true, Report: map[string]any{"reason": "not reproducible: the check command was denied", "memory": "recorded in project:r"}})
		s.FinishCeremony()
		return s.CompleteAssistant(assistant("I cannot reproduce it then."))
	}
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "watch", Repair: &domain.RepairRun{PullRequest: 7, URL: "https://example.test/pr/7"}})
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "merge", State: "COMPLETED", Terminal: true, Report: map[string]any{"memory": "recorded in project:r", "merge_sha": "abc123", "pull_request": 7}, Repair: &domain.RepairRun{PullRequest: 7, URL: "https://example.test/pr/7", MergeSHA: "abc123"}})
	s.FinishCeremony()
	return s.CompleteAssistant(assistant("Merged."))
}
func (w *happyWorkbench) Decide(context.Context, *domain.Session, bool, string, func(Event) error) error {
	w.count(&w.decides, "")
	return errors.New("no merge decision expected")
}
func (w *happyWorkbench) Continue(context.Context, *domain.Session, func(Event) error) error {
	w.count(&w.continues, "")
	return nil
}

// mergeWorkbench parks the ceremony on the person's merge decision.
type mergeWorkbench struct{ workbenchBase }

func (w *mergeWorkbench) Begin(_ context.Context, s *domain.Session, prompt root.Text, _ func(Event) error) error {
	w.count(&w.begins, string(prompt))
	if err := s.BeginTurn(prompt, turnTools()); err != nil {
		return err
	}
	if err := s.SetCeremony(repairRunFor("decide", &domain.RepairRun{Repository: "o/r", PullRequest: 9, URL: "https://example.test/pr/9", Awaiting: domain.AwaitingApproval})); err != nil {
		return err
	}
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "decide", State: "DECIDE", Awaiting: true, Repair: &domain.RepairRun{PullRequest: 9, URL: "https://example.test/pr/9"}})
	return s.CompleteAssistant(assistant("The pull request is green; the person decides."))
}
func (w *mergeWorkbench) Resolve(context.Context, *domain.Session, root.ToolCallID, domain.ToolDecision, func(Event) error) error {
	w.count(&w.resolves, "")
	return errors.New("no approval expected")
}
func (w *mergeWorkbench) Decide(_ context.Context, s *domain.Session, approve bool, reason string, _ func(Event) error) error {
	w.count(&w.decides, "")
	w.mu.Lock()
	w.decisions = append(w.decisions, reason)
	w.mu.Unlock()
	if approve {
		w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "merge", State: "COMPLETED", Terminal: true, Report: map[string]any{"memory": "cause not recorded: KMP refused", "merge_sha": "def456"}, Repair: &domain.RepairRun{PullRequest: 9, URL: "https://example.test/pr/9", MergeSHA: "def456"}})
	} else {
		w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "decide", State: "BLOCKED", Terminal: true, Report: map[string]any{"reason": "declined: " + reason, "memory": "recorded in project:r"}, Repair: &domain.RepairRun{PullRequest: 9, URL: "https://example.test/pr/9"}})
	}
	s.FinishCeremony()
	return nil
}
func (w *mergeWorkbench) Continue(context.Context, *domain.Session, func(Event) error) error {
	w.count(&w.continues, "")
	return nil
}

// failingWorkbench cannot run a turn at all.
type failingWorkbench struct{ workbenchBase }

func (w *failingWorkbench) Begin(context.Context, *domain.Session, root.Text, func(Event) error) error {
	w.count(&w.begins, "")
	return errors.New("model stream: connection reset")
}
func (w *failingWorkbench) Resolve(context.Context, *domain.Session, root.ToolCallID, domain.ToolDecision, func(Event) error) error {
	return nil
}
func (w *failingWorkbench) Decide(context.Context, *domain.Session, bool, string, func(Event) error) error {
	return nil
}
func (w *failingWorkbench) Continue(context.Context, *domain.Session, func(Event) error) error {
	w.count(&w.continues, "")
	return errors.New("model stream: connection reset again")
}

// blockingWorkbench is a console watch that never ends until the console
// stops; Continue after a recovery finds the ceremony merged in MADE.
type blockingWorkbench struct {
	workbenchBase
	started chan struct{}
}

func (w *blockingWorkbench) Begin(ctx context.Context, s *domain.Session, prompt root.Text, _ func(Event) error) error {
	w.count(&w.begins, string(prompt))
	if err := s.BeginTurn(prompt, turnTools()); err != nil {
		return err
	}
	if err := s.SetCeremony(repairRunFor("watch", &domain.RepairRun{Repository: "o/r", PullRequest: 7, URL: "https://example.test/pr/7"})); err != nil {
		return err
	}
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "watch", Repair: &domain.RepairRun{PullRequest: 7, URL: "https://example.test/pr/7"}})
	close(w.started)
	<-ctx.Done()
	return ctx.Err()
}
func (w *blockingWorkbench) Resolve(context.Context, *domain.Session, root.ToolCallID, domain.ToolDecision, func(Event) error) error {
	return nil
}
func (w *blockingWorkbench) Decide(context.Context, *domain.Session, bool, string, func(Event) error) error {
	return nil
}
func (w *blockingWorkbench) Continue(_ context.Context, s *domain.Session, _ func(Event) error) error {
	w.count(&w.continues, "")
	w.observe(CeremonyProgress{Instance: "axlr-child-1", Step: "merge", State: "COMPLETED", Terminal: true, Report: map[string]any{"memory": "recorded in project:r", "merge_sha": "fed789"}, Repair: &domain.RepairRun{PullRequest: 7, URL: "https://example.test/pr/7", MergeSHA: "fed789"}})
	s.FinishCeremony()
	return nil
}

// nudgeWorkbench is a model that keeps ending its turn with the step open.
type nudgeWorkbench struct{ workbenchBase }

func (w *nudgeWorkbench) Begin(_ context.Context, s *domain.Session, prompt root.Text, _ func(Event) error) error {
	w.count(&w.begins, string(prompt))
	if err := s.BeginTurn(prompt, turnTools()); err != nil {
		return err
	}
	if _, live := s.Ceremony(); !live {
		if err := s.SetCeremony(repairRunFor("reproduce", &domain.RepairRun{Repository: "o/r"})); err != nil {
			return err
		}
	}
	return s.CompleteAssistant(assistant("I believe this is done."))
}
func (w *nudgeWorkbench) Resolve(context.Context, *domain.Session, root.ToolCallID, domain.ToolDecision, func(Event) error) error {
	return nil
}
func (w *nudgeWorkbench) Decide(context.Context, *domain.Session, bool, string, func(Event) error) error {
	return nil
}
func (w *nudgeWorkbench) Continue(context.Context, *domain.Session, func(Event) error) error {
	w.count(&w.continues, "")
	return nil
}

type fakeWorkbenches struct {
	mu        sync.Mutex
	workbench RepairWorkbench
	err       error
	clones    []string
}

func (f *fakeWorkbenches) Open(_ context.Context, clone string) (RepairWorkbench, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clones = append(f.clones, clone)
	if f.err != nil {
		return nil, f.err
	}
	return f.workbench, nil
}

type repairRig struct {
	repair    *SelfRepair
	registry  *fakeRegistry
	clones    *fakeClones
	store     *repairStore
	benches   *fakeWorkbenches
	engine    *fakeEngine
	repairsIn string
}

func newRepairRig(t *testing.T, workbench RepairWorkbench) *repairRig {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repairs")
	rig := &repairRig{registry: &fakeRegistry{}, clones: &fakeClones{dir: dir}, store: &repairStore{}, benches: &fakeWorkbenches{workbench: workbench}, engine: &fakeEngine{}, repairsIn: dir}
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "cccccccccccccccccccccccccccccccc"}
	rig.repair = &SelfRepair{Registry: rig.registry, Clones: rig.clones, Workbench: rig.benches, Store: rig.store, Engine: rig.engine,
		Settings: RepairSettings{Repository: "o/r", About: "project:r", Directory: dir}, Build: "0.3.0-test", RunToken: "console-1",
		Now:   func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (domain.SessionID, error) { id := ids[0]; ids = ids[1:]; return domain.SessionID(id), nil }}
	t.Cleanup(rig.repair.Close)
	return rig
}

func mustJSON(text string) root.JSONValue {
	value, err := root.NewJSONObject([]byte(text))
	if err != nil {
		panic(err)
	}
	return value
}

func recurringFailure(t *testing.T) domain.Session {
	t.Helper()
	return parentSession(t, activity{"c1", "local_edit", `{"path":"a.go"}`, outcomeInternal, false}, activity{"c2", "local_edit", `{"path":"a.go"}`, outcomeInternal, false})
}

func waitStatus(t *testing.T, registry *fakeRegistry, id string, statuses ...domain.RepairStatus) domain.RepairRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := registry.find(id); ok {
			for _, status := range statuses {
				if record.Status == status {
					return record
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := registry.find(id)
	t.Fatalf("repair %s is %s (%s), wanted %v", id, record.Status, record.Error, statuses)
	return record
}

func requestRepair(t *testing.T, rig *repairRig, s domain.Session, args string) (map[string]any, error) {
	t.Helper()
	result, err := rig.repair.Request(context.Background(), s, mustObject(t, args))
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(result)
	var out map[string]any
	_ = json.Unmarshal(encoded, &out)
	return out, nil
}

func TestRequestRepairStartsALinkedSessionKeepsTheCheckApprovalAndReportsTheMerge(t *testing.T) {
	bench := &happyWorkbench{}
	rig := newRepairRig(t, bench)
	events := rig.repair.Events()
	parent := recurringFailure(t)
	out, err := requestRepair(t, rig, parent, validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := out["repair"].(string)
	if out["accepted"] != true || !strings.HasPrefix(id, "20261005-1200-localedit-fails") || out["session"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || out["attempt"] != float64(1) {
		t.Fatalf("result: %v", out)
	}
	record := waitStatus(t, rig.registry, id, domain.RepairAwaitingApproval)
	if !strings.Contains(record.Pending, "go test ./...") || record.Step != "reproduce" || record.Instance != "axlr-child-1" || record.Parent != parent.Export().ID || record.Session != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("awaiting record: %+v", record)
	}
	if err := rig.repair.Decide(context.Background(), "unknown", true, ""); err == nil {
		t.Fatal("unknown repair decided")
	}
	if err := rig.repair.Decide(context.Background(), id, true, ""); err != nil {
		t.Fatal(err)
	}
	record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if record.PullRequest != 7 || record.MergeSHA != "abc123" || record.Memory != "recorded in project:r" || record.Pending != "" || record.State != "COMPLETED" {
		t.Fatalf("completed record: %+v", record)
	}
	for _, want := range []string{"merged pull request #7", "abc123", "still runs build 0.3.0-test", "restart", "KMP: recorded in project:r", "MADE instance axlr-child-1"} {
		if !strings.Contains(record.Notice, want) {
			t.Fatalf("notice lacks %q: %s", want, record.Notice)
		}
	}
	notices, err := rig.repair.Drain(context.Background(), parent.Export().ID)
	if err != nil || len(notices) != 1 || notices[0] != record.Notice {
		t.Fatalf("drain: %v %v", notices, err)
	}
	if notices, _ = rig.repair.Drain(context.Background(), parent.Export().ID); len(notices) != 0 {
		t.Fatal("a notice is delivered once")
	}
	child, err := rig.store.Load(context.Background(), record.Session)
	if err != nil || child.Mode() != domain.ModeRepair || string(child.Export().Workspace) != record.Clone || child.Export().Model != parent.Export().Model {
		t.Fatalf("child session: %+v %v", child.Export(), err)
	}
	if request := rig.clones.requests[0]; request.Origin != parent.Export().ID || request.Build != "0.3.0-test" || request.Slug != id || request.About != "project:r" || !strings.Contains(request.Brief, "unexpected nil in replace") {
		t.Fatalf("clone request: %+v", request)
	}
	snapshot := bench.snapshot()
	if snapshot.begins != 1 || snapshot.resolves != 1 || snapshot.resolvedAs[0] != domain.DecisionApprove || snapshot.closed != 1 || !strings.Contains(snapshot.prompts[0], "Expected: the edit is applied") {
		t.Fatalf("workbench calls: %+v", snapshot)
	}
	select {
	case event := <-events:
		if event.Record.ID != id {
			t.Fatalf("event for %s", event.Record.ID)
		}
	default:
		t.Fatal("no event published")
	}
	status, err := rig.repair.Status(context.Background(), parent, mustObject(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(status)
	text := string(encoded)
	if !strings.Contains(text, `"status":"completed"`) || !strings.Contains(text, `"pull_request":7`) || !strings.Contains(text, "needs an updated build") {
		t.Fatalf("status: %s", text)
	}
	if _, err := rig.repair.Status(context.Background(), parent, mustObject(t, `{"repair":""}`)); err == nil {
		t.Fatal("empty repair id accepted")
	}
}

func TestRequestRepairRefusesInsufficientEvidence(t *testing.T) {
	rig := newRepairRig(t, &happyWorkbench{})
	isolated := parentSession(t, activity{"c1", "local_edit", "", outcomeInternal, false}, activity{"c2", "local_exec", "", outcomeExitZero, false})
	oneCall := strings.Replace(validRepairRequest, `"tool_calls":["c1","c2"]`, `"tool_calls":["c1"]`, 1)
	for name, tc := range map[string]struct {
		session domain.Session
		args    string
		want    string
	}{
		"isolated":    {isolated, oneCall, "isolated failure"},
		"unknown":     {isolated, strings.Replace(validRepairRequest, `"c2"`, `"c9"`, 1), "not in this session"},
		"short":       {isolated, `{"description":"it broke","expected":"e","observed":"o","evidence":["x"],"tool_calls":["c1"]}`, "at least 20"},
		"no evidence": {isolated, strings.Replace(oneCall, `"evidence":["call c1 failed with internal_error","call c2 failed identically after a retry"]`, `"evidence":[]`, 1), "evidence needs"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := requestRepair(t, rig, tc.session, tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	if len(rig.registry.records) != 0 || len(rig.clones.requests) != 0 {
		t.Fatal("a refused request must leave no record and no clone")
	}
}

func TestRequestRepairRefusesExternalAndProjectErrors(t *testing.T) {
	rig := newRepairRig(t, &happyWorkbench{})
	for name, tc := range map[string]struct {
		activities []activity
		args       string
		want       string
	}{
		"project":    {[]activity{{"c1", "local_exec", "", outcomeExitOne, false}, {"c2", "local_exec", "", outcomeExitOne, false}}, validRepairRequest, "exited 1"},
		"permission": {[]activity{{"c1", "read", "", outcomePermission, false}, {"c2", "read", "", outcomePermission, false}}, validRepairRequest, "external cause"},
		"rejected":   {[]activity{{"c1", "read", "", outcomeRejected, false}, {"c2", "read", "", outcomeRejected, false}}, validRepairRequest, "rejected by AXLR"},
		"plugin":     {[]activity{{"c1", "kmp_wake", `{"about":"x"}`, outcomePlugin, false}, {"c2", "kmp_wake", `{"about":"x"}`, outcomePlugin, false}}, validRepairRequest, "MCP plugin tool"},
		"denied":     {[]activity{{"c1", "local_edit", "", outcomeDenied, true}, {"c2", "local_edit", "", outcomeDenied, true}}, validRepairRequest, "denied"},
		"provider":   {[]activity{{"c1", "local_edit", "", outcomeInternal, false}, {"c2", "local_edit", "", outcomeInternal, false}}, strings.Replace(validRepairRequest, "twice", "twice; OpenRouter answered 401", 1), "external cause (\"openrouter\")"},
		"timed out":  {[]activity{{"c1", "local_exec", "", outcomeTimedOut, false}, {"c2", "local_exec", "", outcomeTimedOut, false}}, validRepairRequest, "not proof"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := requestRepair(t, rig, parentSession(t, tc.activities...), tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRequestRepairRefusesDuplicatesRecursionAndExhaustedAttempts(t *testing.T) {
	rig := newRepairRig(t, &blockingWorkbench{started: make(chan struct{})})
	parent := recurringFailure(t)
	out, err := requestRepair(t, rig, parent, validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	<-rig.benches.workbench.(*blockingWorkbench).started
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate: %v", err)
	}
	other := parentSession(t, activity{"c1", "axlr_history", `{"message_index":1}`, outcomeHostBroken, false}, activity{"c2", "axlr_history", `{"message_index":1}`, outcomeHostBroken, false})
	if _, err := requestRepair(t, rig, other, validRepairRequest); err == nil || !strings.Contains(err.Error(), "another repair is active") {
		t.Fatalf("concurrency: %v", err)
	}
	repairMode := recurringFailure(t)
	if err := repairMode.SetMode(domain.ModeRepair); err != nil {
		t.Fatal(err)
	}
	if _, err := requestRepair(t, rig, repairMode, validRepairRequest); err == nil || !strings.Contains(err.Error(), "repair session cannot request") {
		t.Fatalf("repair mode: %v", err)
	}
	child, _ := rig.store.Load(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := child.SetMode(domain.ModeNormal); err != nil {
		t.Fatal(err)
	}
	if err := child.BeginTurn("x", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	if err := child.CompleteAssistant(assistant("", root.ToolCall{ID: "c1", Name: "local_edit", Arguments: mustJSON(`{}`)}, root.ToolCall{ID: "c2", Name: "local_edit", Arguments: mustJSON(`{}`)})); err != nil {
		t.Fatal(err)
	}
	for _, id := range []root.ToolCallID{"c1", "c2"} {
		if err := child.RecordToolOutcome(id, domain.DecisionApprove, domain.ToolOutcome{Content: outcomeInternal, IsError: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := child.CompleteAssistant(assistant("done")); err != nil {
		t.Fatal(err)
	}
	if _, err := requestRepair(t, rig, child, validRepairRequest); err == nil || !strings.Contains(err.Error(), "repair clone") {
		t.Fatalf("child workspace: %v", err)
	}
	rig.repair.Close()
	record := waitStatus(t, rig.registry, id, domain.RepairInterrupted)
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("interrupted duplicate: %v", err)
	}
	record.Status, record.Build, record.PullRequest, record.URL, record.MergeSHA = domain.RepairCompleted, "0.3.0-test", 7, "u", "abc"
	_ = rig.registry.Save(context.Background(), record)
	rig.repair.closed = false
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "already repaired") || !strings.Contains(err.Error(), "restart") {
		t.Fatalf("already repaired: %v", err)
	}
	record.Build = "older"
	record.Status = domain.RepairBlocked
	_ = rig.registry.Save(context.Background(), record)
	second := record
	second.ID, second.Session = "second", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_ = rig.registry.Save(context.Background(), second)
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "attempts exhausted") {
		t.Fatalf("attempts: %v", err)
	}
	rig.repair.Settings.MaxAttempts = 3
	rig.repair.Engine = nil
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "MADE is not connected") {
		t.Fatalf("no engine: %v", err)
	}
	rig.repair.Engine = &fakeEngine{ready: ErrCeremonyNotPrepared}
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err == nil || !strings.Contains(err.Error(), "prepare MADE") {
		t.Fatalf("not prepared: %v", err)
	}
}

func TestRequestRepairReportsACloneThatCannotBeMade(t *testing.T) {
	rig := newRepairRig(t, &happyWorkbench{})
	rig.clones.err = errors.New("gh repo clone o/r: exit 4: gh: not logged in")
	parent := recurringFailure(t)
	_, err := requestRepair(t, rig, parent, validRepairRequest)
	if err == nil || !strings.Contains(err.Error(), "no repair session started") || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("clone failure: %v", err)
	}
	if len(rig.registry.records) != 1 || rig.registry.records[0].Status != domain.RepairFailed || !strings.Contains(rig.registry.records[0].Error, "clone:") || rig.registry.records[0].Session != "" {
		t.Fatalf("record: %+v", rig.registry.records)
	}
	notices, _ := rig.repair.Drain(context.Background(), parent.Export().ID)
	if len(notices) != 1 || !strings.Contains(notices[0], "failed") {
		t.Fatalf("notices: %v", notices)
	}
	// The failed attempt started no session, so it does not count as one.
	rig.clones.err = nil
	if _, err := requestRepair(t, rig, parent, validRepairRequest); err != nil {
		t.Fatalf("retry after a clone failure: %v", err)
	}
}

func TestRepairThatFailsDuringTheSessionKeepsTheEvidence(t *testing.T) {
	bench := &failingWorkbench{}
	rig := newRepairRig(t, bench)
	parent := recurringFailure(t)
	out, err := requestRepair(t, rig, parent, validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, out["repair"].(string), domain.RepairFailed)
	if !strings.Contains(record.Error, "connection reset") || record.Clone == "" || record.Session == "" {
		t.Fatalf("failed record: %+v", record)
	}
	if !strings.Contains(record.Notice, "failed") || !strings.Contains(record.Notice, record.Clone) {
		t.Fatalf("notice: %s", record.Notice)
	}
	snapshot := bench.snapshot()
	if snapshot.begins != 1 || snapshot.continues != maxRepairRetries || snapshot.closed != 1 {
		t.Fatalf("retries: %+v", snapshot)
	}
	if _, err := os.Stat(record.Clone); err != nil {
		t.Fatal("the clone must be kept")
	}
}

func TestInterruptedRepairIsRecoveredWithoutRepeatingEffects(t *testing.T) {
	bench := &blockingWorkbench{started: make(chan struct{})}
	rig := newRepairRig(t, bench)
	parent := recurringFailure(t)
	out, err := requestRepair(t, rig, parent, validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	<-bench.started
	record := waitStatus(t, rig.registry, id, domain.RepairRunning)
	if record.Step != "watch" || record.PullRequest != 7 {
		t.Fatalf("live progress not mirrored: %+v", record)
	}
	rig.repair.Close()
	record = waitStatus(t, rig.registry, id, domain.RepairInterrupted)
	if !strings.Contains(record.Notice, "interrupted") || !strings.Contains(record.Error, "watch") {
		t.Fatalf("interrupted record: %+v", record)
	}
	// A new console launch finds the record and the person recovers it.
	again := &SelfRepair{Registry: rig.registry, Clones: rig.clones, Workbench: rig.benches, Store: rig.store, Engine: rig.engine, Settings: rig.repair.Settings, Build: "0.3.0-test", RunToken: "console-2"}
	t.Cleanup(again.Close)
	if err := again.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := again.Recover(context.Background(), "nope"); err == nil {
		t.Fatal("unknown repair recovered")
	}
	if err := again.Recover(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := again.Recover(context.Background(), id); err == nil {
		t.Fatal("a running repair recovered twice")
	}
	record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if record.MergeSHA != "fed789" || record.RunToken != "console-2" || record.PullRequest != 7 {
		t.Fatalf("recovered record: %+v", record)
	}
	snapshot := bench.snapshot()
	if snapshot.begins != 1 || snapshot.continues != 1 {
		t.Fatalf("recovery must continue, not begin again: %+v", snapshot)
	}
	notices, _ := again.Drain(context.Background(), parent.Export().ID)
	if len(notices) != 1 || !strings.Contains(notices[0], "merged pull request #7") {
		t.Fatalf("notices: %v", notices)
	}
}

func TestReconcileMarksRepairsOfAStoppedConsoleInterrupted(t *testing.T) {
	registry := &fakeRegistry{records: []domain.RepairRecord{
		{ID: "stale", Signature: "s", Repository: "o/r", Parent: "0123456789abcdef0123456789abcdef", Status: domain.RepairAwaitingMerge, RunToken: "old", Session: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{ID: "done", Signature: "s", Repository: "o/r", Parent: "0123456789abcdef0123456789abcdef", Status: domain.RepairCompleted, RunToken: "old"},
	}}
	repair := &SelfRepair{Registry: registry, RunToken: "new", Build: "b"}
	if err := repair.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	stale, _ := registry.find("stale")
	done, _ := registry.find("done")
	if stale.Status != domain.RepairInterrupted || !strings.Contains(stale.Notice, "recover") || done.Status != domain.RepairCompleted {
		t.Fatalf("reconciled: %+v %+v", stale, done)
	}
	records, _ := repair.Records(context.Background())
	if len(records) != 2 {
		t.Fatal(records)
	}
}

func TestMergeDecisionWaitsForThePersonAndDeclineBlocks(t *testing.T) {
	for _, approve := range []bool{true, false} {
		bench := &mergeWorkbench{}
		rig := newRepairRig(t, bench)
		parent := recurringFailure(t)
		out, err := requestRepair(t, rig, parent, validRepairRequest)
		if err != nil {
			t.Fatal(err)
		}
		id := out["repair"].(string)
		record := waitStatus(t, rig.registry, id, domain.RepairAwaitingMerge)
		if !strings.Contains(record.Pending, "merge pull request #9") || record.PullRequest != 9 {
			t.Fatalf("awaiting merge: %+v", record)
		}
		if err := rig.repair.Decide(context.Background(), id, false, "  "); err == nil {
			t.Fatal("a decline needs a reason")
		}
		if err := rig.repair.Decide(context.Background(), id, approve, "not tonight"); err != nil {
			t.Fatal(err)
		}
		if approve {
			record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
			if record.MergeSHA != "def456" || !strings.Contains(record.Notice, "KMP: cause not recorded: KMP refused") {
				t.Fatalf("approved: %+v", record)
			}
		} else {
			record = waitStatus(t, rig.registry, id, domain.RepairBlocked)
			if !strings.Contains(record.Error, "declined: not tonight") || !strings.Contains(record.Notice, "BLOCKED") || !strings.Contains(record.Notice, "Pull request #9") || bench.snapshot().decisions[0] != "not tonight" {
				t.Fatalf("declined: %+v", record)
			}
		}
		rig.repair.Close()
	}
}

func TestDeniedCheckCommandEndsTheRepairHonestly(t *testing.T) {
	bench := &happyWorkbench{}
	rig := newRepairRig(t, bench)
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	waitStatus(t, rig.registry, id, domain.RepairAwaitingApproval)
	if err := rig.repair.Decide(context.Background(), id, false, ""); err != nil {
		t.Fatal(err)
	}
	// Without the check the failure cannot be shown; the ceremony ends
	// BLOCKED with the reason and the pull request is never opened.
	record := waitStatus(t, rig.registry, id, domain.RepairBlocked)
	if !strings.Contains(record.Error, "not reproducible") || bench.snapshot().resolvedAs[0] != domain.DecisionDeny || record.PullRequest != 0 || !strings.Contains(record.Notice, "BLOCKED") {
		t.Fatalf("denied: %+v", record)
	}
}

func TestModelThatLeavesTheStepOpenIsNudgedThenStopped(t *testing.T) {
	bench := &nudgeWorkbench{}
	rig := newRepairRig(t, bench)
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, out["repair"].(string), domain.RepairFailed)
	snapshot := bench.snapshot()
	if snapshot.begins != 1+maxRepairNudges || !strings.Contains(snapshot.prompts[1], "Nobody is reading this repair session") || !strings.Contains(record.Error, "step reproduce open") {
		t.Fatalf("nudges: %+v %+v", snapshot, record)
	}
}

func TestRepairNoticeNeverClaimsTheRunningConsoleIsFixed(t *testing.T) {
	record := domain.RepairRecord{ID: "r", Status: domain.RepairCompleted, PullRequest: 3, URL: "u", Repository: "o/r", MergeSHA: "s", Instance: "i"}
	notice := repairNotice(record, "0.2.2")
	if !strings.Contains(notice, "does not contain the fix") || !strings.Contains(notice, "0.2.2") || !strings.Contains(notice, "no memory write") {
		t.Fatal(notice)
	}
	if repairNotice(domain.RepairRecord{Status: domain.RepairRunning}, "b") != "" {
		t.Fatal("a running repair has no notice")
	}
}

func TestRequestRepairFailsWhenTheWorkbenchCannotOpen(t *testing.T) {
	rig := newRepairRig(t, &happyWorkbench{})
	rig.benches.err = errors.New("runtime: root is not a directory")
	parent := recurringFailure(t)
	_, err := requestRepair(t, rig, parent, validRepairRequest)
	if err == nil || !strings.Contains(err.Error(), "open the repair workbench") {
		t.Fatalf("workbench failure: %v", err)
	}
	record := rig.registry.records[0]
	if record.Status != domain.RepairFailed || record.Session == "" || !strings.Contains(record.Notice, "failed") || !strings.Contains(record.Notice, "with session") {
		t.Fatalf("record: %+v", record)
	}
	if len(rig.store.sessions) != 1 {
		t.Fatal("the repair session is kept with its clone")
	}
}

func TestBlockedReasonReadsTheReportAndDefaultsAreUsable(t *testing.T) {
	for _, tc := range []struct {
		report map[string]any
		want   string
	}{
		{map[string]any{"reason": "declined"}, "declined"},
		{map[string]any{"watch": map[string]any{"reason": "did not settle"}}, "did not settle"},
		{map[string]any{"error": "gh: not logged in"}, "gh: not logged in"},
		{map[string]any{"check": map[string]any{"output_tail": "FAIL"}}, "last check output: FAIL"},
		{map[string]any{}, "ended BLOCKED"},
	} {
		if got := blockedReason(tc.report); !strings.Contains(got, tc.want) {
			t.Fatalf("%v: %q", tc.report, got)
		}
	}
	repair := &SelfRepair{}
	id, err := repair.newID()
	if err != nil || len(id) != 32 {
		t.Fatalf("default id: %q %v", id, err)
	}
	if repair.now().IsZero() || repair.lifetime() == nil || repair.maxAttempts() != DefaultMaxRepairAttempts {
		t.Fatal("defaults")
	}
	if err := repair.Recover(context.Background(), "x"); err == nil {
		t.Fatal("unconfigured recover")
	}
	if _, err := repair.Status(context.Background(), turnSession(t), mustObject(t, `{}`)); err == nil {
		t.Fatal("unconfigured status")
	}
	var none *SelfRepair
	none.Close()
	if notices, err := none.Drain(context.Background(), "x"); notices != nil || err != nil {
		t.Fatal("nil coordinator drains nothing")
	}
	if err := none.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}
