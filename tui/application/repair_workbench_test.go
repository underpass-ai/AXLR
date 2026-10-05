package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type progressRecorder struct{ progress []CeremonyProgress }

func (p *progressRecorder) Observe(progress CeremonyProgress) {
	p.progress = append(p.progress, progress)
}

func TestCeremonyObserverSeesStepsWaitsAndTheTerminalReport(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, _, _, _, s := repairDriver(t, forge, 1, 0)
	observer := &progressRecorder{}
	d.Observer = observer
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	var steps []string
	for _, progress := range observer.progress {
		steps = append(steps, progress.Step)
	}
	joined := strings.Join(steps, " ")
	if !strings.Contains(joined, "diagnose repair propose watch decide merge") {
		t.Fatalf("observed steps: %s", joined)
	}
	last := observer.progress[len(observer.progress)-1]
	if !last.Terminal || last.State != "COMPLETED" || last.Report["merge_sha"] != "merge123" || last.Repair == nil || last.Repair.PullRequest != 7 || last.Instance == "" {
		t.Fatalf("terminal progress: %+v", last)
	}
	// The person's merge decision is observed as a wait.
	forge = &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, _, _, _, s = repairDriver(t, forge, 1, 0)
	observer = &progressRecorder{}
	d.Observer = observer
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	last = observer.progress[len(observer.progress)-1]
	if !last.Awaiting || last.State != "DECIDE" || last.Repair.PullRequest != 7 {
		t.Fatalf("awaiting progress: %+v", last)
	}
}

func TestUseCaseWorkbenchContinuesAnInterruptedSessionWithoutReplayingEffects(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	// The console died inside the watch: the session still says repair, MADE
	// says WATCH with the pull request recorded.
	engine.view = CeremonyView{State: "WATCH", Claimable: []string{"watch"}, Outputs: map[string]map[string]any{"propose": {"pull_request": float64(7), "url": "u", "head_sha": "h"}}}
	store := &memoryStore{}
	observer := &progressRecorder{}
	workbench := &UseCaseWorkbench{Start: StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Store: store, Ceremonies: d}}, Agent: AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Ceremonies: d}}}
	workbench.Observe(observer)
	if d.Observer != observer {
		t.Fatal("observer not attached to the driver")
	}
	if err := workbench.Continue(context.Background(), &s, nil); err != nil {
		t.Fatal(err)
	}
	if _, live := s.Ceremony(); live || !forge.merged || len(forge.proposals) != 0 || len(store.states) != 1 {
		t.Fatalf("resume: live=%v merged=%v proposals=%d saves=%d", live, forge.merged, len(forge.proposals), len(store.states))
	}
	if last := observer.progress[len(observer.progress)-1]; !last.Terminal || last.State != "COMPLETED" {
		t.Fatalf("terminal not observed: %+v", last)
	}
	closed := false
	workbench.Closer = func() error { closed = true; return nil }
	if err := workbench.Close(); err != nil || !closed {
		t.Fatal("close")
	}
	if err := (&UseCaseWorkbench{}).Continue(context.Background(), nil, nil); err == nil {
		t.Fatal("continue without a session")
	}
}

func TestUseCaseWorkbenchReturnsPendingCallsToApprovalNotExecution(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", call("c1", "read"))); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseTurn(); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	executed := false
	workbench := &UseCaseWorkbench{Start: StartTurnUseCase{Store: store, Continue: ContinueTurnUseCase{Store: store}}, Agent: AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store}, Tools: toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executed = true
		return domain.ToolOutcome{Content: "ran"}, nil
	})}}
	if err := workbench.Continue(context.Background(), &s, nil); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusApproval || executed || len(s.Pending()) != 1 {
		t.Fatalf("status %s executed=%v", s.Status(), executed)
	}
	complete := turnSession(t)
	if err := workbench.Continue(context.Background(), &complete, nil); err != nil {
		t.Fatal(err)
	}
}

type toolFunc func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error)

func (f toolFunc) Execute(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	return f(ctx, id, args)
}

type noticesFunc func(domain.SessionID) []string

func (f noticesFunc) Drain(_ context.Context, id domain.SessionID) ([]string, error) {
	return f(id), nil
}

func TestStartTurnCarriesRepairNoticesIntoThePrompt(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	var seen string
	model := streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		seen = string(r.Messages[len(r.Messages)-1].Content)
		return assistant("ok"), nil
	})
	notices := noticesFunc(func(id domain.SessionID) []string {
		if id != s.Export().ID {
			t.Fatalf("drained %s", id)
		}
		return []string{"[AXLR] Self-repair r merged pull request #1; restart the console."}
	})
	start := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store}, Notices: notices}
	if err := start.Execute(context.Background(), &s, "hello", ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seen, "hello\n\n[AXLR] Self-repair r merged") {
		t.Fatalf("prompt: %q", seen)
	}
}

type fakeRequestPort struct{ requests, statuses int }

func (f *fakeRequestPort) Request(context.Context, domain.Session, root.JSONValue) (any, error) {
	f.requests++
	return map[string]any{"accepted": true}, nil
}
func (f *fakeRequestPort) Status(context.Context, domain.Session, root.JSONValue) (any, error) {
	f.statuses++
	return map[string]any{"repairs": []any{}}, nil
}

func TestHostToolRoutesRepairRequestsAndRefusesWithoutACoordinator(t *testing.T) {
	s := hostSession(t, "reply")
	for _, op := range []string{domain.HostOperationRequestRepair, domain.HostOperationRepairStatus} {
		out := hostExecute(t, s, op, `{}`)
		if !out.IsError || !strings.Contains(string(out.Content), "not available") {
			t.Fatalf("%s without coordinator: %s", op, out.Content)
		}
	}
	port := &fakeRequestPort{}
	for _, op := range []string{domain.HostOperationRequestRepair, domain.HostOperationRepairStatus} {
		id, _ := domain.NewHostToolIdentity(op)
		out, err := (HostToolUseCase{Repairs: port}).Execute(context.Background(), s, id, hostJSON(t, `{}`))
		if err != nil || out.IsError {
			t.Fatalf("%s: %v %s", op, err, out.Content)
		}
	}
	if port.requests != 1 || port.statuses != 1 {
		t.Fatalf("routing: %+v", port)
	}
}

func TestRepairToolsAreIntrinsicAndHiddenFromRepairSessions(t *testing.T) {
	for _, op := range []string{domain.HostOperationRequestRepair, domain.HostOperationRepairStatus} {
		id, _ := domain.NewHostToolIdentity(op)
		if !automaticallyApproves(nil, id) {
			t.Fatalf("%s needs a card", op)
		}
	}
	s := turnSession(t)
	snapshot := append(turnTools(), HostTools()...)
	if !hasDefinition(SessionTools(s, snapshot), HostRequestRepairName) || !hasDefinition(SessionTools(s, snapshot), HostRepairStatusName) {
		t.Fatal("normal sessions see the repair tools")
	}
	if err := s.SetMode(domain.ModeRepair); err != nil {
		t.Fatal(err)
	}
	tools := SessionTools(s, snapshot)
	if hasDefinition(tools, HostRequestRepairName) || !hasDefinition(tools, HostRepairStatusName) || hasDefinition(tools, HostStepDoneName) {
		t.Fatal("a repair session must not request another repair")
	}
	policy := RepairToolPolicy{AutonomousLocal: true}
	local, _ := domain.NewLocalToolIdentity("exec")
	plugin, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_wake"})
	if !policy.AutoApproves(local) || policy.AutoApproves(plugin) {
		t.Fatal("autonomous local policy")
	}
	strict := RepairToolPolicy{Next: approvesAll{}}
	if strict.AutoApproves(local) != true || !strict.AutoApproves(plugin) {
		t.Fatal("delegation to the configured policy")
	}
	if (RepairToolPolicy{}).AutoApproves(local) {
		t.Fatal("no policy approves nothing")
	}
}

type approvesAll struct{}

func (approvesAll) AutoApproves(domain.ToolIdentity) bool { return true }

func hasDefinition(tools []root.ToolDefinition, name root.ToolName) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func TestModelGuidanceExplainsSelfRepairOutsideRepairSessions(t *testing.T) {
	s := turnSession(t)
	if !strings.Contains(string(modelHostGuidance(&s).Content), "axlr_request_repair") {
		t.Fatal("normal sessions are told how to request a repair")
	}
	if err := s.SetMode(domain.ModeRepair); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(modelHostGuidance(&s).Content), "axlr_request_repair") {
		t.Fatal("repair sessions are not")
	}
	var schema map[string]any
	for _, tool := range HostTools() {
		if tool.Definition.Name == HostRequestRepairName {
			if err := json.Unmarshal(tool.Definition.Parameters.Bytes(), &schema); err != nil {
				t.Fatal(err)
			}
		}
	}
	if required, _ := schema["required"].([]any); len(required) != 5 {
		t.Fatalf("schema: %v", schema)
	}
	if errors.Is(ErrCeremonyNotPrepared, nil) {
		t.Fatal("unreachable")
	}
}

func TestUseCaseWorkbenchDelegatesBeginResolveAndDecide(t *testing.T) {
	store := &memoryStore{}
	model := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("ok"), nil
	})
	continuation := ContinueTurnUseCase{Models: model, Store: store}
	workbench := &UseCaseWorkbench{Start: StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: continuation}, Resolver: ResolveToolUseCase{Store: store, Continue: continuation}, Agent: AgentTurnUseCase{Continue: continuation}}
	s := turnSession(t)
	if err := workbench.Begin(context.Background(), &s, "repair it", nil); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusComplete || len(s.Messages()) != 2 {
		t.Fatalf("begin: %s %d", s.Status(), len(s.Messages()))
	}
	pending := turnSession(t)
	if err := pending.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := pending.CompleteAssistant(assistant("", call("c1", "read"))); err != nil {
		t.Fatal(err)
	}
	if err := workbench.Resolve(context.Background(), &pending, "c1", domain.DecisionDeny, nil); err != nil {
		t.Fatal(err)
	}
	if pending.Status() != domain.StatusComplete || pending.Export().Activity[0].Decision != domain.DecisionDeny {
		t.Fatalf("resolve: %s %+v", pending.Status(), pending.Export().Activity[0])
	}
	// The merge decision reaches the ceremony driver and the turn that follows.
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, _, _, _, repairSession := repairDriver(t, forge, 1, 0)
	if err := d.Begin(context.Background(), &repairSession, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &repairSession, reproduceArgs)
	step(t, d, &repairSession, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &repairSession, `{"summary":"s","summary_en":"e"}`)
	if run, _ := repairSession.Ceremony(); !run.AwaitingPerson() {
		t.Fatal("the merge should wait for the person")
	}
	continuation.Ceremonies = d
	workbench.Start.Continue = continuation
	if err := workbench.Decide(context.Background(), &repairSession, false, "not tonight", nil); err != nil {
		t.Fatal(err)
	}
	if _, live := repairSession.Ceremony(); live || forge.merged || repairSession.Status() != domain.StatusComplete {
		t.Fatalf("decide: live=%v merged=%v status=%s", live, forge.merged, repairSession.Status())
	}
}
