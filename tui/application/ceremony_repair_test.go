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

func init() {
	for trigger, state := range map[string]string{
		"proposed": "WATCH", "propose_failed": "BLOCKED", "checks_passed": "DECIDE", "checks_failed": "REPAIR", "watch_blocked": "BLOCKED",
		"merge_automatic": "MERGE", "merge_approved": "MERGE", "merge_declined": "BLOCKED", "merged": "COMPLETED", "merge_failed": "BLOCKED",
	} {
		fakeTransitions[trigger] = state
	}
}

// fakeForge scripts the pull request's life: each Status call pops one status.
type fakeForge struct {
	proposals            []RepairProposal
	statuses             []PullRequestStatus
	updates              int
	merged               bool
	proposeErr, mergeErr error
}

func (f *fakeForge) Propose(_ context.Context, p RepairProposal) (PullRequest, error) {
	f.proposals = append(f.proposals, p)
	if f.proposeErr != nil {
		return PullRequest{}, f.proposeErr
	}
	number := p.Number
	if number == 0 {
		number = 7
	}
	return PullRequest{Number: number, URL: "https://example.test/pr/7", HeadSHA: "head" + string(rune('0'+len(f.proposals)))}, nil
}
func (f *fakeForge) Status(context.Context, string, int) (PullRequestStatus, error) {
	if len(f.statuses) == 0 {
		return PullRequestStatus{}, errors.New("no scripted status")
	}
	status := f.statuses[0]
	f.statuses = f.statuses[1:]
	return status, nil
}
func (f *fakeForge) UpdateBranch(context.Context, string, int) error { f.updates++; return nil }
func (f *fakeForge) Merge(context.Context, string, int) (string, error) {
	if f.mergeErr != nil {
		return "", f.mergeErr
	}
	f.merged = true
	return "merge123", nil
}

type fakeRepairFiles struct{ marker string }

func (f fakeRepairFiles) Read(_ context.Context, path string, _ int) ([]byte, bool, error) {
	if path == RepairMarker && f.marker != "" {
		return []byte(f.marker), true, nil
	}
	return nil, false, nil
}
func (fakeRepairFiles) Write(context.Context, string, []byte) error { return nil }
func (fakeRepairFiles) MakeDir(context.Context, string) error       { return nil }

type mergeApprover struct{ guards []string }

func (f *mergeApprover) ApproveGuard(_ context.Context, _, guard string) error {
	f.guards = append(f.guards, guard)
	return nil
}

const repairMarker = `{"version":1,"repository":"underpass-ai/axlr-repair-lab","base":"main","slug":"20261005-0200-hola-sale-1","brief":"hola sale 1","about":"project:axlr-repair-lab","created":"2026-10-05T02:00:00Z"}`

func repairDriver(t *testing.T, forge *fakeForge, exits ...int) (*CeremonyDriver, *fakeEngine, *fakeChecks, *fakeMemory, domain.Session) {
	t.Helper()
	engine, checks := &fakeEngine{}, &fakeChecks{exits: exits}
	memory := &fakeMemory{wake: "- project:axlr-repair-lab:entry:decision:known-cause (decision): the split bug", refs: []string{"project:axlr-repair-lab:entry:decision:known-cause"}}
	d := &CeremonyDriver{Engine: engine, Checks: checks, Memory: memory, Files: fakeRepairFiles{marker: repairMarker}, Forge: forge, Approver: &mergeApprover{},
		Now:   func() time.Time { return time.Unix(1, 0) },
		Sleep: func(context.Context, time.Duration) error { return nil }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeRepair); err != nil {
		t.Fatal(err)
	}
	return d, engine, checks, memory, s
}

func TestRepairRefusesAWorkspaceWithoutTheMarker(t *testing.T) {
	d, _, _, _, s := repairDriver(t, &fakeForge{})
	d.Files = fakeRepairFiles{}
	err := d.Begin(context.Background(), &s, "hola sale 1")
	if err == nil || !strings.Contains(err.Error(), "axlr-tui --repair") {
		t.Fatalf("begin without marker: %v", err)
	}
}

func TestRepairBeginsWithFocusedRecallAndRepositoryInputs(t *testing.T) {
	d, engine, _, memory, s := repairDriver(t, &fakeForge{})
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Ceremony()
	if run.Definition != "axlr_repair" || run.About != "project:axlr-repair-lab" || run.Repair == nil || run.Repair.Branch != "repair/20261005-0200-hola-sale-1" {
		t.Fatalf("run: %+v", run)
	}
	if len(memory.intents) != 1 || memory.intents[0] != "hola sale 1" {
		t.Fatalf("wake intent: %v", memory.intents)
	}
	if len(run.Repair.WakeRefs) != 1 || !strings.Contains(Instruction(run), "the split bug") {
		t.Fatalf("recall not carried: %+v", run.Repair.WakeRefs)
	}
	if engine.calls[0] != "start axlr_repair 1.0 about=project:axlr-repair-lab" {
		t.Fatalf("start: %v", engine.calls)
	}
}

func TestRepairRunsToAutomaticMergeAndRecordsCauseAndOutcome(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{
		{State: "OPEN", MergeState: "UNKNOWN", Pending: 3},
		{State: "OPEN", MergeState: "CLEAN", Passed: 3, HeadSHA: "head1"},
	}}
	d, engine, checks, memory, s := repairDriver(t, forge, 1, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	if r := step(t, d, &s, reproduceArgs); r["next_step"] != "diagnose" {
		t.Fatalf("reproduce: %v", r)
	}
	r := step(t, d, &s, `{"root_cause":"split(\" \")","evidence":"probe","proposed_fix":"split()","connect_to":[{"ref":"project:axlr-repair-lab:entry:decision:known-cause","rel":"restates","why":"same bug"},{"ref":"project:other:entry:x","rel":"follows","why":"guess"}]}`)
	if r["next_step"] != "repair" || r["memory"] != "cause recorded in project:axlr-repair-lab with 1 links" {
		t.Fatalf("diagnose: %v", r)
	}
	if refused, _ := r["links_refused"].([]any); len(refused) != 1 || refused[0] != "project:other:entry:x (follows)" {
		t.Fatalf("refused links: %v", r["links_refused"])
	}
	if len(memory.records) != 1 || memory.records[0].Kind != "error_path" || len(memory.records[0].Links) != 1 {
		t.Fatalf("cause record: %+v", memory.records)
	}
	if r := step(t, d, &s, `{"summary":"use split()"}`); r["accepted"] != false || !strings.Contains(r["error"].(string), "summary_en") {
		t.Fatalf("repair without summary_en: %v", r)
	}
	r = step(t, d, &s, `{"summary":"use split()","summary_en":"The splitter now splits on any whitespace."}`)
	if r["ceremony"] != "COMPLETED" || r["merge_sha"] != "merge123" || r["pull_request"] != float64(7) {
		t.Fatalf("repair chain: %v", r)
	}
	if _, live := s.Ceremony(); live || s.Mode() != domain.ModeNormal {
		t.Fatal("ceremony should be over")
	}
	joined := strings.Join(engine.calls, " | ")
	for _, want := range []string{"claim propose 1", "complete propose", "transition proposed", "claim watch 1", "complete watch", "transition checks_passed", "claim decide 1", "transition merge_automatic", "claim merge 1", "transition merged"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
	if len(forge.proposals) != 1 || forge.proposals[0].Branch != "repair/20261005-0200-hola-sale-1" || !strings.Contains(forge.proposals[0].Body, "split(\" \")") || !strings.Contains(forge.proposals[0].Trailer, "Repaired-by: AXLR axlr_repair 1.0") {
		t.Fatalf("proposal: %+v", forge.proposals)
	}
	if !forge.merged || len(checks.runs) != 2 {
		t.Fatalf("merged=%v checks=%d", forge.merged, len(checks.runs))
	}
	outcome := memory.records[len(memory.records)-1]
	if outcome.Kind != "success_path" || !strings.Contains(outcome.Summary, "pull request #7") || !strings.Contains(outcome.Summary, "merge123") {
		t.Fatalf("outcome: %+v", outcome)
	}
	if len(outcome.Links) != 1 || outcome.Links[0].Ref != "project:axlr-repair-lab:entry:error_path:"+memory.records[0].ID || outcome.Links[0].Rel != "follows" {
		t.Fatalf("outcome link: %+v", outcome.Links)
	}
	if forge.proposals[0].Title != "Repair: hola sale 1" {
		t.Fatalf("title: %q", forge.proposals[0].Title)
	}
	if labels := memory.labels[len(memory.labels)-1]; labels["pull_request"][0] != "7" || labels["repair"][0] != "20261005-0200-hola-sale-1" {
		t.Fatalf("labels: %v", labels)
	}
	for _, out := range engine.completed {
		if decision, ok := out["decision"]; ok && decision != "automatic" {
			t.Fatalf("decide: %v", out)
		}
	}
}

func TestRepairRedRoundGoesBackToRepairWithTheFailingChecks(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{
		{State: "OPEN", MergeState: "BLOCKED", Passed: 2, Failed: []string{"go: failure https://ci/1"}},
		{State: "OPEN", MergeState: "BEHIND", Passed: 3},
		{State: "OPEN", MergeState: "CLEAN", Passed: 3},
	}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	r := step(t, d, &s, `{"summary":"s","summary_en":"s en"}`)
	if r["next_step"] != "repair" || r["state"] != "REPAIR" {
		t.Fatalf("red round: %v", r)
	}
	run, _ := s.Ceremony()
	if run.Repair.Rounds != 1 || !strings.Contains(run.Repair.Feedback, "go: failure") || run.Repair.PullRequest != 7 || !strings.Contains(Instruction(run), "came back red on pull request #7") {
		t.Fatalf("round state: %+v", run.Repair)
	}
	r = step(t, d, &s, `{"summary":"s2","summary_en":"s2 en"}`)
	if r["ceremony"] != "COMPLETED" {
		t.Fatalf("second round: %v", r)
	}
	if forge.updates != 1 || len(forge.proposals) != 2 || forge.proposals[1].Number != 7 {
		t.Fatalf("forge: updates=%d proposals=%+v", forge.updates, forge.proposals)
	}
	joined := strings.Join(engine.calls, " | ")
	if !strings.Contains(joined, "claim repair c1") || !strings.Contains(joined, "claim propose c1") {
		t.Fatalf("second-round claim keys: %s", joined)
	}
}

func TestRepairBlocksAfterTheRoundsAreExhausted(t *testing.T) {
	red := PullRequestStatus{State: "OPEN", MergeState: "BLOCKED", Passed: 1, Failed: []string{"go: failure"}}
	forge := &fakeForge{statuses: []PullRequestStatus{red, red, red}}
	d, _, _, memory, s := repairDriver(t, forge, 1, 0, 0, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	r := step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if r["ceremony"] != "BLOCKED" {
		t.Fatalf("exhausted rounds: %v", r)
	}
	outcome := memory.records[len(memory.records)-1]
	if outcome.Kind != "observation" || !strings.Contains(outcome.Summary, "checks failed after 2 repair rounds") {
		t.Fatalf("outcome: %+v", outcome)
	}
}

func TestRepairWaitsForThePersonAndMergesOnApproval(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	approver := d.Approver.(*mergeApprover)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	r := step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if r["next_step"] != "decide" || !strings.Contains(r["instruction"].(string), "/repair") {
		t.Fatalf("await: %v", r)
	}
	run, _ := s.Ceremony()
	if !run.AwaitingPerson() || !CanReturn(run) {
		t.Fatalf("not awaiting: %+v", run.Repair)
	}
	if r := step(t, d, &s, `{"summary":"again"}`); r["accepted"] != false {
		t.Fatalf("model should be refused while the person decides: %v", r)
	}
	result, err := d.Approve(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	_ = json.Unmarshal([]byte(result.Outcome.Content), &report)
	if !result.Accepted || result.Run != nil || report["ceremony"] != "COMPLETED" || !forge.merged {
		t.Fatalf("approve: %+v %v", result, report)
	}
	if len(approver.guards) != 1 || approver.guards[0] != "person_approves" {
		t.Fatalf("guard: %v", approver.guards)
	}
	if joined := strings.Join(engine.calls, " | "); !strings.Contains(joined, "transition merge_approved") {
		t.Fatal(joined)
	}
}

func TestRepairDecideWaitsForThePersonOnALongLease(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	last := engine.leases[len(engine.leases)-1]
	if last != presentLease {
		t.Fatalf("decide lease %v, want %v", last, presentLease)
	}
}

func TestRepairReconcileRecoversThePullRequestFromMADE(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	// The model resends repair after an interrupted watch: MADE refuses the
	// completion because the instance is in WATCH, where propose already
	// recorded the pull request.
	engine.failCompletes = 1
	engine.view = CeremonyView{State: "WATCH", Claimable: []string{"watch"}, Outputs: map[string]map[string]any{"propose": {"pull_request": float64(7), "url": "u", "head_sha": "h"}}}
	r := step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if r["ceremony"] != "COMPLETED" || !forge.merged {
		t.Fatalf("reconciled watch: %v", r)
	}
	if len(forge.proposals) != 0 {
		t.Fatalf("propose must not run again: %+v", forge.proposals)
	}
}

func TestRepairDeclineLeavesThePullRequestOpenAndBlocks(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, memory, s := repairDriver(t, forge, 1, 0)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if _, err := d.Return(context.Background(), s, ""); err == nil {
		t.Fatal("decline needs a reason")
	}
	result, err := d.Return(context.Background(), s, "not tonight")
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	_ = json.Unmarshal([]byte(result.Outcome.Content), &report)
	if report["ceremony"] != "BLOCKED" || forge.merged {
		t.Fatalf("decline: %v merged=%v", report, forge.merged)
	}
	if joined := strings.Join(engine.calls, " | "); !strings.Contains(joined, "transition merge_declined") {
		t.Fatal(joined)
	}
	if outcome := memory.records[len(memory.records)-1]; outcome.Kind != "observation" {
		t.Fatalf("outcome: %+v", outcome)
	}
}

func TestRepairProposeFailureBlocksHonestly(t *testing.T) {
	forge := &fakeForge{proposeErr: errors.New("gh: not logged in")}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	r := step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if r["ceremony"] != "BLOCKED" {
		t.Fatalf("propose failure: %v", r)
	}
	if !strings.Contains(strings.Join(engine.calls, " | "), "transition propose_failed") {
		t.Fatal(engine.calls)
	}
	for _, out := range engine.completed {
		if proposed, ok := out["proposed"]; ok && (proposed != false || !strings.Contains(out["error"].(string), "not logged in")) {
			t.Fatalf("propose output: %v", out)
		}
	}
}

func TestRepairWatchStallBlocksAtTheDeadline(t *testing.T) {
	pending := PullRequestStatus{State: "OPEN", MergeState: "UNKNOWN", Pending: 2}
	forge := &fakeForge{statuses: []PullRequestStatus{pending, pending, pending, pending}}
	d, _, _, _, s := repairDriver(t, forge, 1, 0)
	clock := time.Unix(1, 0)
	d.Now = func() time.Time { return clock }
	d.Sleep = func(context.Context, time.Duration) error { clock = clock.Add(20 * time.Minute); return nil }
	d.RepairPolicy = RepairPolicy{AutoMerge: true, WatchDeadline: 45 * time.Minute}
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	r := step(t, d, &s, `{"summary":"s","summary_en":"e"}`)
	if r["ceremony"] != "BLOCKED" || !strings.Contains(r["watch"].(map[string]any)["reason"].(string), "did not settle") {
		t.Fatalf("stall: %v", r)
	}
}

func TestRepairResumeReentersTheWatchFromMADE(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	d, engine, _, _, s := repairDriver(t, forge, 1, 0)
	d.RepairPolicy.AutoMerge = true
	if err := d.Begin(context.Background(), &s, "hola sale 1"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, reproduceArgs)
	step(t, d, &s, `{"root_cause":"c","evidence":"e","proposed_fix":"f"}`)
	// The session still says repair (the watch was cancelled before its
	// result was saved); MADE says WATCH with the propose output recorded.
	engine.view = CeremonyView{State: "WATCH", Claimable: []string{"watch"}, Outputs: map[string]map[string]any{"propose": {"pull_request": float64(7), "url": "https://example.test/pr/7", "head_sha": "head1"}}}
	result, ok, err := d.Resume(context.Background(), s)
	if err != nil || !ok {
		t.Fatalf("resume: ok=%v err=%v", ok, err)
	}
	var report map[string]any
	_ = json.Unmarshal([]byte(result.Outcome.Content), &report)
	if report["ceremony"] != "COMPLETED" || report["resumed"] != "repair" || !forge.merged {
		t.Fatalf("resume result: %v", report)
	}
	if !strings.Contains(strings.Join(engine.calls, " | "), "claim watch 1") {
		t.Fatal(engine.calls)
	}
}

func TestRepairModeRefusesModelMemoryWrites(t *testing.T) {
	id, err := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"})
	if err != nil {
		t.Fatal(err)
	}
	verdict, reason := domain.ModeRepair.Judge(id, mustObject(t, `{}`))
	if verdict != domain.VerdictDeny || !strings.Contains(reason, "connect_to") {
		t.Fatalf("verdict %v %q", verdict, reason)
	}
}
