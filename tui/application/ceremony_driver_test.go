package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type fakeEngine struct {
	ready           error
	state           string
	calls           []string
	completed       []map[string]any
	failCompletes   int
	failTransitions int
	view            CeremonyView
	leases          []time.Duration
	definition      string
}

var fakeTransitions = map[string]string{
	"reproduced": "DIAGNOSE", "not_reproducible": "BLOCKED", "reproduce_exhausted": "BLOCKED",
	"diagnosed": "REPAIR", "repaired": "INTEGRATE", "repair_exhausted": "BLOCKED",
	"briefed": "BUILD", "verified": "INTEGRATE", "build_exhausted": "BLOCKED", "integrated": "COMPLETED",
	"decomposed": "APPROVAL", "decompose_exhausted": "BLOCKED", "approved": "READY", "approved_automatically": "READY", "returned": "DECOMPOSE", "declined": "BLOCKED",
	"test_first": "RED", "test_present": "GREEN", "test_failing": "GREEN", "untestable": "BLOCKED", "red_exhausted": "BLOCKED", "check_passing": "HANDBACK", "green_exhausted": "BLOCKED", "handed_back": "DONE",
}

func (f *fakeEngine) Ready(context.Context, string, string) error { return f.ready }
func (f *fakeEngine) Start(_ context.Context, definition, version, instance string, inputs map[string]string) error {
	f.calls = append(f.calls, "start "+definition+" "+version+" about="+inputs["memory_about"])
	f.definition = definition
	return nil
}
func (f *fakeEngine) Claim(_ context.Context, _, step, key string, lease time.Duration) (string, error) {
	f.leases = append(f.leases, lease)
	f.calls = append(f.calls, "claim "+step+" "+key[strings.LastIndex(key, ":")+1:])
	return "fence-" + step, nil
}
func (f *fakeEngine) Complete(_ context.Context, _, step, fence string, output map[string]any) error {
	if fence != "fence-"+step {
		return errors.New("stale fence")
	}
	if f.failCompletes > 0 {
		f.failCompletes--
		return errors.New("step is not in progress")
	}
	f.calls = append(f.calls, "complete "+step)
	f.completed = append(f.completed, output)
	return nil
}
func (f *fakeEngine) Transition(_ context.Context, _, trigger string) (string, error) {
	f.calls = append(f.calls, "transition "+trigger)
	if f.failTransitions > 0 {
		f.failTransitions--
		return "", errors.New("connection reset")
	}
	f.state = fakeTransitions[trigger]
	if f.definition == "axlr_repair" && trigger == "repaired" {
		f.state = "PROPOSE" // the repair ceremony proposes instead of integrating
	}
	if f.definition == "axlr_improve" {
		// The improve ceremony proposes after build and builds again after
		// a red check round.
		switch trigger {
		case "verified":
			f.state = "PROPOSE"
		case "checks_failed":
			f.state = "BUILD"
		}
	}
	return f.state, nil
}
func (f *fakeEngine) Cancel(_ context.Context, _, reason string) error {
	f.calls = append(f.calls, "cancel "+reason)
	return nil
}
func (f *fakeEngine) Inspect(context.Context, string) (CeremonyView, error) {
	f.calls = append(f.calls, "inspect")
	return f.view, nil
}

type fakeChecks struct {
	exits []int
	runs  []domain.CheckCommand
	// status is what git status --porcelain prints: a clean clone by default;
	// statusStderr is what it adds on stderr while still exiting 0.
	status, statusStderr string
	// git, when set, answers the other git commands (rev-parse HEAD in a
	// repository without commits, say).
	git *CheckResult
}

func (f *fakeChecks) Run(_ context.Context, command domain.CheckCommand) (CheckResult, error) {
	f.runs = append(f.runs, command)
	if command.Program == "git" && len(command.Args) > 0 && command.Args[0] == "status" {
		return CheckResult{Ran: true, Output: f.status + f.statusStderr, Stdout: f.status}, nil
	}
	if command.Program == "git" {
		if f.git != nil {
			return *f.git, nil
		}
		return CheckResult{Ran: true, Output: "abc123\n", Stdout: "abc123\n"}, nil
	}
	exit := 0
	if len(f.exits) > 0 {
		exit, f.exits = f.exits[0], f.exits[1:]
	}
	if exit == -1 {
		return CheckResult{ExitCode: -1, Output: "program not found"}, nil
	}
	return CheckResult{Ran: true, ExitCode: exit, Output: "tail"}, nil
}

type fakeMemory struct {
	about, summary string
	// wake and refs are what WakeFocused returns; intents records the
	// intents it was asked with; records keeps every linked write.
	wake    string
	refs    []string
	intents []string
	records []MemoryRecord
	labels  []map[string][]string
	fail    error
}

func (f *fakeMemory) Wake(context.Context, string) (string, error) { return "", nil }
func (f *fakeMemory) WakeFocused(_ context.Context, _, intent string) (string, []string, error) {
	f.intents = append(f.intents, intent)
	return f.wake, f.refs, nil
}
func (f *fakeMemory) Record(_ context.Context, about string, labels map[string][]string, _, summary, _ string) error {
	f.about, f.summary = about, summary+" labels="+strings.Join(labels["ceremony"], ",")
	return nil
}
func (f *fakeMemory) RecordLinked(_ context.Context, about string, labels map[string][]string, record MemoryRecord) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	f.about, f.summary = about, record.Summary
	f.records = append(f.records, record)
	f.labels = append(f.labels, labels)
	return about + ":entry:" + record.Kind + ":" + record.ID, nil
}

func debugSession(t *testing.T) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDebug); err != nil {
		t.Fatal(err)
	}
	return s
}

func step(t *testing.T, d *CeremonyDriver, s *domain.Session, args string) map[string]any {
	t.Helper()
	result, err := d.StepDone(context.Background(), *s, mustObject(t, args))
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(result.Outcome.Content), &report); err != nil {
		t.Fatal(err)
	}
	if result.Accepted {
		if result.Run == nil {
			s.FinishCeremony()
		} else if err := s.SetCeremony(*result.Run); err != nil {
			t.Fatal(err)
		}
	}
	return report
}

const reproduceArgs = `{"check_command":{"program":"python3","args":["-m","unittest"]},"expected":"2 hola","observed":"1 hola"}`

func TestDebugCeremonyRunsEndToEndOnConsoleChecks(t *testing.T) {
	engine, checks, memory := &fakeEngine{}, &fakeChecks{exits: []int{1, 1, 0}}, &fakeMemory{}
	d := &CeremonyDriver{Engine: engine, Checks: checks, Memory: memory, Now: func() time.Time { return time.Unix(1, 0) }}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	if len(memory.intents) != 1 || memory.intents[0] != "hola sale 1" {
		t.Fatalf("the user's request must focus the recall: %v", memory.intents)
	}
	if run, _ := s.Ceremony(); run.Step != "reproduce" || run.About != "ws:"+string(s.Export().ID) {
		t.Fatalf("begin: %+v", run)
	}
	if r := step(t, d, &s, reproduceArgs); r["next_step"] != "diagnose" {
		t.Fatalf("reproduce: %v", r)
	}
	if r := step(t, d, &s, `{"root_cause":"split(\" \")","evidence":"probe","proposed_fix":"split()"}`); r["next_step"] != "repair" {
		t.Fatalf("diagnose: %v", r)
	}
	if r := step(t, d, &s, `{"summary":"first try"}`); r["next_step"] != "repair" {
		t.Fatalf("failed repair should repeat: %v", r)
	}
	if run, _ := s.Ceremony(); run.Iteration != 2 {
		t.Fatalf("repeat did not advance the iteration: %d", run.Iteration)
	}
	if r := step(t, d, &s, `{"summary":"use split()"}`); r["next_step"] != "integrate" {
		t.Fatalf("repair: %v", r)
	}
	if r := step(t, d, &s, `{"report":"arreglado","summary_en":"Fixed word splitting."}`); r["ceremony"] != "COMPLETED" {
		t.Fatalf("integrate: %v", r)
	}
	if _, live := s.Ceremony(); live || s.Mode() != domain.ModeNormal {
		t.Fatal("finished ceremony still live")
	}
	if memory.about != "ws:"+string(s.Export().ID) || !strings.Contains(memory.summary, "COMPLETED") {
		t.Fatalf("outcome not recorded: %+v", memory)
	}
	for _, run := range checks.runs[:3] {
		if run.Program != "python3" || strings.Join(run.Args, " ") != "-m unittest" {
			t.Fatalf("console ran something other than the approved command: %+v", run)
		}
	}
	reproduced := engine.completed[0]
	if reproduced["reproduced"] != true || reproduced["settled"] != true {
		t.Fatalf("console did not set the guard fields: %v", reproduced)
	}
}

// unbornHead is git rev-parse HEAD in a repository without commits: it
// echoes the argument on stdout, explains on stderr and exits 128.
var unbornHead = CheckResult{Ran: true, ExitCode: 128, Stdout: "HEAD\n",
	Output: "HEAD\nfatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.\nUse '--' to separate paths from revisions, like this:\n'git <command> [<revision>...] -- [<file>...]'\n"}

// Integrate records git's answer as evidence only when git answered: a
// failed rev-parse, or a warning git status printed on stderr, is not a
// revision or a dirty file.
func TestIntegrateRecordsOnlyWhatGitAnswered(t *testing.T) {
	engine := &fakeEngine{}
	checks := &fakeChecks{exits: []int{1, 0}, git: &unbornHead, statusStderr: "warning: could not open directory 'cache/': Permission denied\n"}
	d := &CeremonyDriver{Engine: engine, Checks: checks, Now: func() time.Time { return time.Unix(1, 0) }}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{reproduceArgs, `{"root_cause":"r","evidence":"e","proposed_fix":"f"}`, `{"summary":"fixed"}`} {
		step(t, d, &s, args)
	}
	r := step(t, d, &s, `{"report":"hecho","summary_en":"Fixed."}`)
	integrated := engine.completed[len(engine.completed)-1]
	if r["ceremony"] != "COMPLETED" || integrated["revision"] != "" || integrated["dirty"] != "" || r["revision"] != "" {
		t.Fatalf("integrate recorded revision %q and dirty %q", integrated["revision"], integrated["dirty"])
	}
}

func TestStepRefusalsLeaveMADEUntouched(t *testing.T) {
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	before := len(engine.calls)
	for _, args := range []string{`{"summary":"skipping ahead"}`, `{"expected":"a","observed":"b"}`, `{"repaired":true}`} {
		result, err := d.StepDone(context.Background(), s, mustObject(t, args))
		if err != nil || result.Accepted || !result.Outcome.IsError {
			t.Fatalf("%s accepted: %+v %v", args, result, err)
		}
	}
	if len(engine.calls) != before {
		t.Fatalf("refusals reached MADE: %v", engine.calls[before:])
	}
}

func TestReproductionThatNeverFailsEndsBlocked(t *testing.T) {
	d := &CeremonyDriver{Engine: &fakeEngine{}, Checks: &fakeChecks{exits: []int{0, 0, 0}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	var last map[string]any
	for i := 0; i < 3; i++ {
		last = step(t, d, &s, reproduceArgs)
	}
	if last["ceremony"] != "BLOCKED" || s.Mode() != domain.ModeNormal {
		t.Fatalf("exhausted reproduction: %v", last)
	}
}

func TestBeginRefusesWithoutThePublishedDefinition(t *testing.T) {
	d := &CeremonyDriver{Engine: &fakeEngine{ready: ErrCeremonyNotPrepared}, Checks: &fakeChecks{}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); !errors.Is(err, ErrCeremonyNotPrepared) {
		t.Fatalf("got %v", err)
	}
	var none *CeremonyDriver
	if err := none.Begin(context.Background(), &s, "x"); err == nil {
		t.Fatal("began without MADE")
	}
}

func TestOnlyANewCheckCommandNeedsTheUser(t *testing.T) {
	d := &CeremonyDriver{Engine: &fakeEngine{}, Checks: &fakeChecks{exits: []int{1}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	stepDone, _ := domain.NewHostToolIdentity(domain.HostOperationStepDone)
	if approvesInSession(allowEverything{}, s, stepDone, mustObject(t, reproduceArgs)) {
		t.Fatal("a new check command was approved automatically")
	}
	step(t, d, &s, reproduceArgs)
	if !approvesInSession(nil, s, stepDone, mustObject(t, `{"root_cause":"a","evidence":"b","proposed_fix":"c"}`)) {
		t.Fatal("a step without a command waited for the user")
	}
	if !approvesInSession(nil, s, stepDone, mustObject(t, `{"root_cause":"a","evidence":"b","proposed_fix":"c","check_command":{"program":"sh"}}`)) {
		t.Fatal("diagnose, which runs no command, asked to approve one")
	}
	run, _ := s.Ceremony()
	run.Step = "repair"
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	if !approvesInSession(nil, s, stepDone, mustObject(t, `{"summary":"x","check_command":{"program":"sh","args":["-c","true"]}}`)) {
		t.Fatal("repair asked to approve a command it will ignore")
	}
}

func TestAcceptedStepUpdatesTheSessionAndRestartsTheBudget(t *testing.T) {
	s, store := pendingCall(t, domain.ModeDebug, "axlr_step_done", reproduceArgs)
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{1}}}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "axlr-i", Step: "reproduce", Iteration: 1, Fence: "fence-reproduce"}); err != nil {
		t.Fatal(err)
	}
	u := ResolveToolUseCase{Store: store, Continue: ContinueTurnUseCase{Store: store, Ceremonies: d}}
	if err := u.resolveOne(context.Background(), &s, "call-1", domain.DecisionApprove, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	run, live := s.Ceremony()
	if !live || run.Step != "diagnose" || !run.Check.Equal(domain.CheckCommand{Program: "python3", Args: []string{"-m", "unittest"}}) {
		t.Fatalf("session run not advanced: %+v", run)
	}
	if run.BudgetBase != s.Export().TurnCallCount || s.Export().TurnCallCount == 0 {
		t.Fatalf("budget not restarted: base %d count %d", run.BudgetBase, s.Export().TurnCallCount)
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatalf("a session with a restarted budget no longer restores: %v", err)
	}
	var saved bool
	for _, state := range store.states {
		if state.Ceremony != nil && state.Ceremony.Step == "diagnose" {
			saved = true
		}
	}
	if !saved {
		t.Fatal("advanced run was not saved")
	}
	_ = root.Text("")
}

func TestACommandThatNeverRanDoesNotReproduceOrRepair(t *testing.T) {
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{-1}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	before := len(engine.calls)
	result, err := d.StepDone(context.Background(), s, mustObject(t, reproduceArgs))
	if err != nil || result.Accepted || len(engine.calls) != before {
		t.Fatalf("a command that never ran reproduced the failure: %+v", result)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "repair", Iteration: 1, Fence: "fence-repair", Check: domain.CheckCommand{Program: "nope"}}); err != nil {
		t.Fatal(err)
	}
	d.Checks = &fakeChecks{exits: []int{-1}}
	if r := step(t, d, &s, `{"summary":"x"}`); r["next_step"] != "repair" || !strings.Contains(r["feedback"].(string), "did not run") {
		t.Fatalf("a check that never ran counted as passing: %v", r)
	}
}

func TestADeliveryBriefNeedsACheckThatRuns(t *testing.T) {
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{-1, 1}}}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDelivery); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin(context.Background(), &s, "add --top"); err != nil {
		t.Fatal(err)
	}
	brief := `{"criteria":"--top N limits output","scope":"wc.py","check_command":{"program":"python","args":["-m","unittest"]}}`
	if result, _ := d.StepDone(context.Background(), s, mustObject(t, brief)); result.Accepted {
		t.Fatal("a baseline that never ran was accepted")
	}
	if r := step(t, d, &s, brief); r["next_step"] != "build" {
		t.Fatalf("a failing baseline should still open the build: %v", r)
	}
}

func TestAFailedTransitionIsReconciledFromMADE(t *testing.T) {
	engine := &fakeEngine{failTransitions: 1, view: CeremonyView{State: "REPRODUCE", Enabled: []string{"reproduced"}}}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{1}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	r := step(t, d, &s, reproduceArgs)
	if r["next_step"] != "diagnose" || r["reconciled"] == nil {
		t.Fatalf("not reconciled: %v", r)
	}
	if got := strings.Join(engine.calls[len(engine.calls)-4:], ","); got != "transition reproduced,inspect,transition reproduced,claim diagnose 1" {
		t.Fatalf("unexpected recovery: %s", got)
	}
}

func TestAResumedSessionCatchesUpWithMADE(t *testing.T) {
	engine := &fakeEngine{failCompletes: 1, view: CeremonyView{State: "DIAGNOSE", Claimable: []string{"diagnose"}}}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{1}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	reminded, _ := s.Ceremony()
	reminded.Reminded = true
	if err := s.SetCeremony(reminded); err != nil {
		t.Fatal(err)
	}
	if r := step(t, d, &s, reproduceArgs); r["next_step"] != "diagnose" {
		t.Fatalf("session did not catch up: %v", r)
	}
	if run, _ := s.Ceremony(); run.Step != "diagnose" || run.Iteration != 1 || run.Fence != "fence-diagnose" || run.Reminded {
		t.Fatalf("run not re-synced: %+v", run)
	}
}

func TestAFailureMADECannotExplainIsReported(t *testing.T) {
	engine := &fakeEngine{failCompletes: 1, view: CeremonyView{State: "REPRODUCE"}}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{1}}}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.StepDone(context.Background(), s, mustObject(t, reproduceArgs)); err == nil || !strings.Contains(err.Error(), "not in progress") {
		t.Fatalf("original failure hidden: %v", err)
	}
}

func TestRepairKeepsTheCheckApprovedInReproduce(t *testing.T) {
	checks := &fakeChecks{exits: []int{1, 0}}
	d := &CeremonyDriver{Engine: &fakeEngine{}, Checks: checks}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "x"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"a","evidence":"b","proposed_fix":"c"}`)
	r := step(t, d, &s, `{"summary":"fixed","check_command":{"program":"true"}}`)
	if r["next_step"] != "integrate" {
		t.Fatalf("repair: %v", r)
	}
	last := checks.runs[len(checks.runs)-1]
	if last.Program != "python3" || strings.Join(last.Args, " ") != "-m unittest" {
		t.Fatalf("repair ran the model's new command instead of the approved one: %+v", last)
	}
	if run, _ := s.Ceremony(); run.Check.Program != "python3" {
		t.Fatalf("approved check replaced: %+v", run.Check)
	}
}

func TestAnOpenStepGetsOneVisibleReminder(t *testing.T) {
	s := debugSession(t)
	if err := s.BeginTurn("arregla", localSnapshot(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "build", Iteration: 1, Fence: "f"}); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	generations := 0
	model := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		generations++
		return assistant("all tests pass"), nil
	})
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Models: model}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if generations != 2 {
		t.Fatalf("expected one reminder and two generations, got %d", generations)
	}
	reminders := 0
	for _, m := range s.Messages() {
		if m.Role == root.RoleUser && strings.HasPrefix(string(m.Content), "[AXLR] The build step") {
			reminders++
		}
	}
	run, _ := s.Ceremony()
	if reminders != 1 || !run.Reminded {
		t.Fatalf("reminders %d reminded %v", reminders, run.Reminded)
	}
}
