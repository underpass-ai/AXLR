package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func init() {
	fakeTransitions["not_feasible"] = "BLOCKED"
	fakeTransitions["brief_exhausted"] = "BLOCKED"
}

const improveMarker = `{"version":1,"repository":"underpass-ai/axlr-repair-lab","base":"main","slug":"20261008-1200-show-the-log","brief":"show the log\nto the agent","about":"project:axlr-repair-lab","kind":"improve","created":"2026-10-08T12:00:00Z"}`

const improveBriefArgs = `{"criteria":"axlr logs prints the last lines","scope":"tui/cmd","check_command":{"program":"go","args":["test","./cmd/..."]}}`

// improveDriver is repairDriver in improve mode, on an improvement clone.
func improveDriver(t *testing.T, forge *fakeForge, exits ...int) (*CeremonyDriver, *fakeEngine, *fakeChecks, *fakeMemory, domain.Session) {
	t.Helper()
	d, engine, checks, memory, s := repairDriver(t, forge, exits...)
	d.Files = fakeRepairFiles{marker: improveMarker}
	if err := s.SetMode(domain.ModeImprove); err != nil {
		t.Fatal(err)
	}
	return d, engine, checks, memory, s
}

func TestImproveAndRepairRefuseEachOthersClones(t *testing.T) {
	d, _, _, _, s := improveDriver(t, &fakeForge{})
	d.Files = fakeRepairFiles{}
	if err := d.Begin(context.Background(), &s, "show the log"); err == nil || !strings.Contains(err.Error(), "axlr-tui --improve") {
		t.Fatalf("begin without marker: %v", err)
	}
	d.Files = fakeRepairFiles{marker: repairMarker}
	if err := d.Begin(context.Background(), &s, "show the log"); err == nil || !strings.Contains(err.Error(), "not an improvement clone: its marker was written for the other ceremony") || !strings.Contains(err.Error(), "axlr-tui --improve") {
		t.Fatalf("improve in a repair clone: %v", err)
	}
	r, _, _, _, repair := repairDriver(t, &fakeForge{})
	r.Files = fakeRepairFiles{marker: improveMarker}
	if err := r.Begin(context.Background(), &repair, "hola sale 1"); err == nil || !strings.Contains(err.Error(), "not a repair clone") || !strings.Contains(err.Error(), "axlr-tui --repair") {
		t.Fatalf("repair in an improvement clone: %v", err)
	}
}

func TestImproveBriefNeedsAFailingCheckThenWaitsForThePersonsMerge(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "CLEAN", Passed: 3}}}
	// The first brief's check passes (no improvement shown), the second
	// fails, build makes it pass.
	d, engine, checks, memory, s := improveDriver(t, forge, 0, 1, 0)
	d.RepairPolicy.AutoMerge = true // repair.auto_merge does not apply
	if err := d.Begin(context.Background(), &s, "show the log\nto the agent"); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Ceremony()
	if run.Definition != "axlr_improve" || run.Step != "brief" || run.Repair == nil || !run.Repair.Improvement || run.Repair.Branch != "improve/20261008-1200-show-the-log" {
		t.Fatalf("run: %+v %+v", run, run.Repair)
	}
	if text := Instruction(run); !strings.Contains(text, "must exit non-zero") || !strings.Contains(text, "attempt 1 of 3") || !strings.Contains(text, "feasible=false") {
		t.Fatalf("brief instruction: %s", text)
	}
	if len(memory.intents) != 1 || memory.intents[0] != "show the log\nto the agent" {
		t.Fatalf("wake intent: %v", memory.intents)
	}
	if engine.calls[0] != "start axlr_improve 1.0 about=project:axlr-repair-lab" {
		t.Fatalf("start: %v", engine.calls)
	}
	if r := step(t, d, &s, `{"criteria":"c","check_command":{"program":"go"}}`); r["accepted"] != false {
		t.Fatalf("brief without scope: %v", r)
	}
	r := step(t, d, &s, improveBriefArgs)
	if r["next_step"] != "brief" || !strings.Contains(r["feedback"].(string), "exits 0 before any change") {
		t.Fatalf("passing baseline: %v", r)
	}
	r = step(t, d, &s, improveBriefArgs)
	if r["next_step"] != "build" || !strings.Contains(r["instruction"].(string), "summary_en") {
		t.Fatalf("failing baseline: %v", r)
	}
	if r := step(t, d, &s, `{"summary":"adds axlr logs"}`); r["accepted"] != false || !strings.Contains(r["error"].(string), "build needs summary and summary_en") {
		t.Fatalf("build without summary_en: %v", r)
	}
	r = step(t, d, &s, `{"summary":"adds axlr logs","summary_en":"The console prints its log tail."}`)
	if r["next_step"] != "decide" || !strings.Contains(r["instruction"].(string), "/improve") {
		t.Fatalf("await: %v", r)
	}
	run, _ = s.Ceremony()
	if !run.AwaitingPerson() || run.Repair.Criteria != "axlr logs prints the last lines" || run.Repair.Scope != "tui/cmd" {
		t.Fatalf("not awaiting: %+v", run.Repair)
	}
	if last := engine.leases[len(engine.leases)-1]; last != presentLease {
		t.Fatalf("decide lease %v", last)
	}
	result, err := d.Approve(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	_ = json.Unmarshal([]byte(result.Outcome.Content), &report)
	if !result.Accepted || report["ceremony"] != "COMPLETED" || !forge.merged {
		t.Fatalf("approve: %+v %v", result, report)
	}
	joined := strings.Join(engine.calls, " | ")
	if strings.Contains(joined, "merge_automatic") || !strings.Contains(joined, "transition merge_approved") || !strings.Contains(joined, "claim brief 2") {
		t.Fatalf("calls: %s", joined)
	}
	var checked, statuses int
	for _, run := range checks.runs {
		if run.Program == "git" {
			statuses++
		} else {
			checked++
		}
	}
	if checked != 3 || statuses != 2 {
		t.Fatalf("checks: %v", checks.runs)
	}
	p := forge.proposals[0]
	if p.Branch != "improve/20261008-1200-show-the-log" || p.Title != "Improve: show the log" || !strings.HasPrefix(p.Trailer, "Improved-by: AXLR axlr_improve 1.0 ") {
		t.Fatalf("proposal: %+v", p)
	}
	for _, want := range []string{"Improvement made by the AXLR console", "## Criteria\n\naxlr logs prints the last lines", "## Scope\n\ntui/cmd", "The console prints its log tail.", "`go test ./cmd/...` failed before the change and passed after it"} {
		if !strings.Contains(p.Body, want) {
			t.Fatalf("body misses %q:\n%s", want, p.Body)
		}
	}
	if len(memory.records) != 1 {
		t.Fatalf("an improvement writes only its outcome: %+v", memory.records)
	}
	outcome := memory.records[0]
	if outcome.Kind != "success_path" || !strings.HasPrefix(outcome.Summary, `Improvement "20261008-1200-show-the-log"`) || !strings.Contains(outcome.Summary, "Criteria: axlr logs") {
		t.Fatalf("outcome: %+v", outcome)
	}
	if labels := memory.labels[len(memory.labels)-1]; labels["improvement"][0] != "20261008-1200-show-the-log" || labels["repair"] != nil {
		t.Fatalf("labels: %v", labels)
	}
}

func TestImproveBriefRefusesAClonePastItsBaseline(t *testing.T) {
	d, engine, checks, _, s := improveDriver(t, &fakeForge{}, 1)
	if err := d.Begin(context.Background(), &s, "show the log"); err != nil {
		t.Fatal(err)
	}
	checks.status = " M tui/cmd/axlr-tui/run.go\n"
	r := step(t, d, &s, improveBriefArgs)
	if r["accepted"] != false || !strings.Contains(r["error"].(string), "the clone already has changes ( M tui/cmd/axlr-tui/run.go)") {
		t.Fatalf("dirty clone: %v", r)
	}
	if len(checks.runs) != 1 || strings.Contains(strings.Join(engine.calls, " | "), "complete brief") {
		t.Fatalf("the check must not run on a changed clone: %v %v", checks.runs, engine.calls)
	}
	checks.status = ""
	if r := step(t, d, &s, improveBriefArgs); r["next_step"] != "build" {
		t.Fatalf("clean clone: %v", r)
	}
}

func TestImproveNotFeasibleBlocksWithTheReason(t *testing.T) {
	d, engine, checks, memory, s := improveDriver(t, &fakeForge{})
	if err := d.Begin(context.Background(), &s, "show the log"); err != nil {
		t.Fatal(err)
	}
	if r := step(t, d, &s, `{"feasible":false}`); r["accepted"] != false || !strings.Contains(r["error"].(string), "observed") {
		t.Fatalf("not feasible without a reason: %v", r)
	}
	r := step(t, d, &s, `{"feasible":false,"observed":"axlr logs already exists"}`)
	if r["ceremony"] != "BLOCKED" || len(checks.runs) != 0 {
		t.Fatalf("not feasible: %v checks=%v", r, checks.runs)
	}
	if !strings.Contains(strings.Join(engine.calls, " | "), "transition not_feasible") {
		t.Fatal(engine.calls)
	}
	if outcome := memory.records[0]; outcome.Kind != "observation" || !strings.Contains(outcome.Summary, "Not feasible: axlr logs already exists") {
		t.Fatalf("outcome: %+v", outcome)
	}
}

func TestImproveBlocksWhenNoBriefShowsTheImprovementMissing(t *testing.T) {
	d, engine, _, _, s := improveDriver(t, &fakeForge{}, 0, 0, 0)
	if err := d.Begin(context.Background(), &s, "show the log"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, improveBriefArgs)
	step(t, d, &s, improveBriefArgs)
	r := step(t, d, &s, improveBriefArgs)
	if r["ceremony"] != "BLOCKED" || !strings.Contains(strings.Join(engine.calls, " | "), "transition brief_exhausted") {
		t.Fatalf("exhausted briefs: %v %v", r, engine.calls)
	}
}

func TestImproveRedRoundGoesBackToBuildWithTheFailingChecks(t *testing.T) {
	forge := &fakeForge{statuses: []PullRequestStatus{
		{State: "OPEN", MergeState: "BLOCKED", Passed: 2, Failed: []string{"go: failure https://ci/1"}},
		{State: "OPEN", MergeState: "CLEAN", Passed: 3},
	}}
	d, engine, _, _, s := improveDriver(t, forge, 1, 0, 0)
	if err := d.Begin(context.Background(), &s, "show the log"); err != nil {
		t.Fatal(err)
	}
	step(t, d, &s, improveBriefArgs)
	r := step(t, d, &s, `{"summary":"s","summary_en":"s en"}`)
	if r["next_step"] != "build" || r["state"] != "BUILD" {
		t.Fatalf("red round: %v", r)
	}
	run, _ := s.Ceremony()
	if run.Repair.Rounds != 1 || !strings.Contains(Instruction(run), "came back red on pull request #7") {
		t.Fatalf("round state: %+v", run.Repair)
	}
	r = step(t, d, &s, `{"summary":"s2","summary_en":"s2 en"}`)
	if r["next_step"] != "decide" {
		t.Fatalf("second round: %v", r)
	}
	joined := strings.Join(engine.calls, " | ")
	if !strings.Contains(joined, "claim build c1") || !strings.Contains(joined, "claim propose c1") || len(forge.proposals) != 2 || forge.proposals[1].Number != 7 {
		t.Fatalf("second round: %s %+v", joined, forge.proposals)
	}
}

func TestImproveModeKeepsMemoryAndRepairRequestsToTheConsole(t *testing.T) {
	id, err := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"})
	if err != nil {
		t.Fatal(err)
	}
	if verdict, reason := domain.ModeImprove.Judge(id, mustObject(t, `{}`)); verdict != domain.VerdictDeny || !strings.Contains(reason, "improve mode") {
		t.Fatalf("verdict %v %q", verdict, reason)
	}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeImprove); err != nil {
		t.Fatal(err)
	}
	if hasDefinition(SessionTools(s, append(turnTools(), HostTools()...)), HostRequestRepairName) {
		t.Fatal("an improvement session must not request a repair")
	}
	if text := string(modelHostGuidance(&s).Content); !strings.Contains(text, "Mode: improve.") || strings.Contains(text, "Self-repair:") {
		t.Fatalf("guidance: %s", text)
	}
}
