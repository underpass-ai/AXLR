package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type memoryPlans struct{ records []domain.PlanRecord }

func (m *memoryPlans) Load(context.Context) ([]domain.PlanRecord, error) {
	return append([]domain.PlanRecord(nil), m.records...), nil
}
func (m *memoryPlans) Save(_ context.Context, record domain.PlanRecord) error {
	for i := range m.records {
		if m.records[i].ID == record.ID {
			m.records[i] = record
			return nil
		}
	}
	m.records = append(m.records, record)
	return nil
}

type planApprover struct{ guards []string }

func (a *planApprover) ApproveGuard(_ context.Context, _, guard string) error {
	a.guards = append(a.guards, guard)
	return nil
}

const textstatSource = "// Package textstat counts things in text.\npackage textstat\n\nimport \"strings\"\n\n// WordCount returns the number of words in s.\nfunc WordCount(s string) int {\n\treturn len(strings.Split(s, \" \"))\n}\n"

func planDriver(t *testing.T) (*CeremonyDriver, *fakeEngine, *memoryPlans, *planApprover, domain.Session) {
	t.Helper()
	engine, plans, approver := &fakeEngine{}, &memoryPlans{}, &planApprover{}
	files := &fakeFiles{files: map[string][]byte{"textstat.go": []byte(textstatSource), "textstat_test.go": []byte("package textstat\n")}}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Files: files, Plans: plans, Approver: approver, Now: func() time.Time { return time.Unix(1, 0) }, Plan: PlanSettings{Planner: "z-ai/glm-5.3-flash"}}
	s := turnSession(t)
	if err := s.SetMode(domain.ModePlan); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin(context.Background(), &s, "Count words and lines in textstat"); err != nil {
		t.Fatal(err)
	}
	return d, engine, plans, approver, s
}

func planArgs(t *testing.T, tasks string) string {
	t.Helper()
	return `{"tasks":` + tasks + `,"e2e_check":{"program":"go","args":["test","./..."]},"interfaces":"WordCount(s string) int; LineCount(s string) int","summary_en":"Two tasks: fix WordCount, add LineCount."}`
}

const goodTasks = `[
 {"id":"wordcount","goal":"WordCount treats any whitespace run as one separator.","scope":["textstat.go","textstat_test.go"],"context":[{"path":"textstat.go","line":8,"quote":"return len(strings.Split(s, \" \"))"}],"unit_check":{"program":"go","args":["test","-run","TestWordCount","./..."]},"test_first":true},
 {"id":"linecount","goal":"Add LineCount.","scope":["lines.go","lines_test.go"],"context":[{"path":"textstat.go","line":7,"quote":"func WordCount(s string) int {"}],"unit_check":{"program":"go","args":["test","-run","TestLineCount","./..."]},"depends_on":["wordcount"]}
]`

func TestPlanVerificationNamesEveryDefect(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	bad := `[
 {"id":"Bad ID","goal":"x","scope":["../etc/passwd"],"unit_check":{"program":"go"}},
 {"id":"a","goal":"a","scope":["x.go"],"unit_check":{"program":"go"},"depends_on":["b"]},
 {"id":"b","goal":"b","scope":["x.go"],"unit_check":{"program":"go"},"depends_on":["a"]},
 {"id":"c","goal":"c","scope":["y.go"],"unit_check":{"program":"go"},"depends_on":["ghost"]}
]`
	report := step(t, d, &s, planArgs(t, bad))
	text, _ := json.Marshal(report["defects"])
	for _, want := range []string{"must be a slug", "outside the workspace", "form a cycle", "unknown task \\\"ghost\\\""} {
		if !strings.Contains(string(text), want) {
			t.Errorf("defects miss %q: %s", want, text)
		}
	}
	if report["next_step"] != "decompose" || report["feedback"] == nil {
		t.Fatalf("a defective plan must repeat decompose: %v", report)
	}
	run, _ := s.Ceremony()
	if run.Iteration != 2 || len(run.Plan.Defects) == 0 || !strings.Contains(Instruction(run), "Defects of the last proposal") {
		t.Fatalf("the next round does not carry the defects: %+v", run.Plan)
	}
}

func TestPlanRefusesTestFirstOverTestsOnly(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	testsOnly := strings.Replace(goodTasks, `"scope":["lines.go","lines_test.go"]`, `"scope":["lines_test.go"],"test_first":true`, 1)
	report := step(t, d, &s, planArgs(t, testsOnly))
	text, _ := json.Marshal(report["defects"])
	if !strings.Contains(string(text), "task linecount: test_first needs a file to change") {
		t.Fatalf("test_first over tests only must be a defect: %s", text)
	}
	if strings.Contains(string(text), "task wordcount: test_first") {
		t.Fatalf("a test_first task with its code in scope is fine: %s", text)
	}
}

// The mirror case: a test-first task whose scope holds no test cannot
// write the failing test (14 October 2026: add-max had only calc/calc.go,
// and calc/calc_test.go belonged to add-testmax in the same wave).
func TestPlanRefusesTestFirstWithoutATest(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	codeOnly := strings.Replace(goodTasks, `"scope":["textstat.go","textstat_test.go"]`, `"scope":["textstat.go"]`, 1)
	report := step(t, d, &s, planArgs(t, codeOnly))
	text, _ := json.Marshal(report["defects"])
	if !strings.Contains(string(text), "task wordcount: test_first needs a test to write first, but its scope holds no test file; put the test and the code it tests in one task's scope, or set test_first false") {
		t.Fatalf("test_first without a test must be a defect: %s", text)
	}
	if strings.Contains(string(text), "task linecount: test_first") {
		t.Fatalf("a task without test_first is fine: %s", text)
	}
}

func TestPlanCitationsMustQuoteTheLine(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	wrong := strings.Replace(goodTasks, `"line":8`, `"line":3`, 1)
	report := step(t, d, &s, planArgs(t, wrong))
	text, _ := json.Marshal(report["defects"])
	if !strings.Contains(string(text), "textstat.go:3 does not contain the quote") {
		t.Fatalf("citation defect: %s", text)
	}
}

func TestPlanRunsToReadyWithThePersonsApproval(t *testing.T) {
	d, engine, plans, approver, s := planDriver(t)
	run, _ := s.Ceremony()
	if run.Model != "z-ai/glm-5.3-flash" || run.Plan == nil || plans.records[0].Status != domain.PlanDecomposing {
		t.Fatalf("begin: %+v %+v", run, plans.records)
	}
	if !planNeedsApproval(s, mustObject(t, planArgs(t, goodTasks))) {
		t.Fatal("the plan's commands skipped the approval card")
	}
	report := step(t, d, &s, planArgs(t, goodTasks))
	if report["next_step"] != "present" {
		t.Fatalf("verified plan: %v", report)
	}
	record := plans.records[0]
	if record.Status != domain.PlanAwaitingApproval || len(record.Tasks) != 2 || record.Waves != 2 || record.Tasks[1].Wave != 2 {
		t.Fatalf("record: %+v", record)
	}
	if record.Tasks[1].New == nil || !strings.Contains(record.Tasks[0].Pack, "--- textstat.go:1-9 ---") || !strings.Contains(record.Tasks[0].Pack, "Test first") {
		t.Fatalf("pack: %q new=%v", record.Tasks[0].Pack, record.Tasks[1].New)
	}
	run, _ = s.Ceremony()
	if !run.AwaitingPerson() || !CanReturn(run) {
		t.Fatalf("not awaiting the person: %+v", run.Plan)
	}
	result, err := d.Approve(context.Background(), s)
	if err != nil || !result.Accepted || result.Run != nil {
		t.Fatalf("approve: %+v %v", result, err)
	}
	if strings.Join(approver.guards, ",") != "person_approves" || !strings.Contains(strings.Join(engine.calls, "|"), "transition approved") {
		t.Fatalf("guard or transition missing: %v %v", approver.guards, engine.calls)
	}
	if plans.records[0].Status != domain.PlanReady || plans.records[0].Decision != "approve" {
		t.Fatalf("final record: %+v", plans.records[0])
	}
}

func TestPlanReturnAndDecline(t *testing.T) {
	d, engine, plans, _, s := planDriver(t)
	step(t, d, &s, planArgs(t, goodTasks))
	result, err := d.Return(context.Background(), s, "split wordcount into two")
	if err != nil || !result.Accepted || result.Run == nil || result.Run.Step != "decompose" || result.Run.Plan.Returns != 1 {
		t.Fatalf("return: %+v %v", result, err)
	}
	if err := s.SetCeremony(*result.Run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Instruction(*result.Run), "split wordcount into two") || !strings.Contains(strings.Join(engine.calls, "|"), "claim decompose r1") {
		t.Fatalf("the return does not reach the planner: %v", engine.calls)
	}
	step(t, d, &s, planArgs(t, goodTasks))
	declined, err := d.DeclinePlan(context.Background(), s, "not now")
	if err != nil || !declined.Accepted || declined.Run != nil || plans.records[0].Status != domain.PlanDeclined {
		t.Fatalf("decline: %+v %v %+v", declined, err, plans.records[0])
	}
}

func TestPlanAutoApprovalSkipsThePerson(t *testing.T) {
	d, engine, plans, approver, s := planDriver(t)
	d.Plan.AutoApprove = true
	report := step(t, d, &s, planArgs(t, goodTasks))
	if report["ceremony"] != "READY" || len(approver.guards) != 0 || plans.records[0].Decision != "automatic" || !strings.Contains(strings.Join(engine.calls, "|"), "transition approved_automatically") {
		t.Fatalf("auto approval: %v %v %+v", report, engine.calls, plans.records[0])
	}
}

func TestThePlannerModelAnswersWhileThePlanIsLive(t *testing.T) {
	_, _, _, _, s := planDriver(t)
	if err := s.BeginTurn("plan it", turnTools()); err != nil {
		t.Fatal(err)
	}
	var asked root.ModelID
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		asked = r.Model
		return assistant("thinking"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if asked != "z-ai/glm-5.3-flash" || s.Export().Model != "test/model" {
		t.Fatalf("asked %s; session model %s", asked, s.Export().Model)
	}
}

func TestThePlannerSeesOnlyWhatDecomposeNeeds(t *testing.T) {
	_, _, _, _, s := planDriver(t)
	local := []domain.AvailableTool{}
	for _, name := range []string{"local_read", "local_write", "local_edit", "local_exec"} {
		id, _ := domain.NewLocalToolIdentity(strings.TrimPrefix(name, "local_"))
		schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
		local = append(local, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName(name), Parameters: schema}})
	}
	var names []string
	for _, tool := range SessionTools(s, append(local, HostTools()...)) {
		names = append(names, string(tool.Name))
	}
	if strings.Join(names, ",") != "axlr_history,axlr_step_done,local_exec,local_read" {
		t.Fatalf("planner tools = %v", names)
	}
	guidance := string(modelHostGuidance(&s, engineParagraphs{}).Content)
	if strings.Contains(guidance, "axlr_call_tool") || strings.Contains(guidance, "Self-repair") || !strings.Contains(guidance, "Mode: plan") || !strings.Contains(guidance, "atomic tasks") {
		t.Fatalf("planner guidance: %s", guidance)
	}
	if err := s.BeginTurn("plan it", append(local, HostTools()...)); err != nil {
		t.Fatal(err)
	}
	skill := root.ToolCall{ID: "k1", Name: HostSkillName, Arguments: mustObject(t, `{"plugin":"axlr","skill":"x"}`)}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{skill}}}); err != nil {
		t.Fatal(err)
	}
	if err := compactRefusal(s, s.Pending()[0]); err == nil {
		t.Fatal("the planner called a hidden host tool")
	}
}

func TestApprovingAPlanAsksTheModelNothingMore(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	step(t, d, &s, planArgs(t, goodTasks))
	asked := 0
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: &memoryStore{}, Continue: ContinueTurnUseCase{Store: &memoryStore{}, Ceremonies: d, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		asked++
		return assistant("I'll start on the tasks myself"), nil
	})}}
	if err := u.Decide(context.Background(), &s, true, "", ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if asked != 0 || s.Mode() != domain.ModeNormal {
		t.Fatalf("the planner was asked again (%d) or the mode stayed %s", asked, s.Mode())
	}
}

// On 14 October 2026 the planner called local_edit twice before it saw
// that decompose only reads: its instruction starts with its role and the
// tools the step offers, in the system prompt and on its reminder.
func TestThePlannerIsToldItsRoleAndToolsFirst(t *testing.T) {
	run := domain.CeremonyRun{Definition: "axlr_plan", Version: "1.0", Step: "decompose"}
	for _, text := range []string{baseInstruction(run), string(stepReminder(run))} {
		at := strings.Index(text, "Your role: the planner. This step only reads")
		if at < 0 || at > strings.Index(text, "Read the repository") || !strings.Contains(text, "local_write and local_edit are not offered") {
			t.Fatalf("instruction %q", text)
		}
	}
}

// The test-first checks guessed tests by a base name containing "test" or
// "spec": latest.go, contest.go and inspector.go passed for tests and a
// Rust task, whose tests live in the source file, was refused (desk
// review of 14 October 2026). Each language's own naming decides now, and
// a Rust source may hold the failing test.
func TestTestFilesAreRecognisedByTheirLanguagesNaming(t *testing.T) {
	tests := []string{"calc/calc_test.go", "pkg/test_parser.py", "pkg/parser_test.py", "conftest.py", "src/app.test.ts", "src/app.spec.js", "web/Button.test.tsx",
		"src/test/java/calc/CalcTest.java", "app/CalcTests.kt", "app/CalcIT.java", "lib/calc_spec.rb", "test/calc_test.rb", "tests/integration.rs", "__tests__/app.js",
		"CalcTests.cs", "calc_unittest.cc", "test_calc.c", "calc_test.exs", "check.bats"}
	for _, p := range tests {
		if !testFilePath(p) || !mayHoldTests(p) {
			t.Errorf("%s is a test file", p)
		}
	}
	code := []string{"latest.go", "contest.go", "inspector.go", "calc/testing.go", "attestation.py", "spectrum.ts", "Contest.java", "Testament.java", "test.go", "README.md"}
	for _, p := range code {
		if testFilePath(p) {
			t.Errorf("%s is not a test file", p)
		}
	}
	if testFilePath("src/lib.rs") || !mayHoldTests("src/lib.rs") {
		t.Error("a Rust source holds its unit tests")
	}
	// A test-first Rust task with only its source in scope is a plan; a Go
	// task whose only "test" is latest.go is not.
	d, _, _, _, s := planDriver(t)
	rust := strings.Replace(goodTasks, `"scope":["textstat.go","textstat_test.go"]`, `"scope":["src/lib.rs"]`, 1)
	rust = strings.Replace(rust, `"scope":["lines.go","lines_test.go"]`, `"scope":["latest.go"],"test_first":true`, 1)
	report := step(t, d, &s, planArgs(t, rust))
	text, _ := json.Marshal(report["defects"])
	if strings.Contains(string(text), "task wordcount: test_first") || !strings.Contains(string(text), "task linecount: test_first needs a test to write first") {
		t.Fatalf("defects %s", text)
	}
}

// On 14 October 2026, with the planner's role in the system prompt,
// glm-5.3-flash still read the person's /plan brief as an order and called
// local_edit twice: the brief itself carries a note that it is a brief.
func TestAPlanBriefSaysItIsABriefOnThePersonsMessage(t *testing.T) {
	engine, plans := &fakeEngine{}, &memoryPlans{}
	files := &fakeFiles{files: map[string][]byte{"textstat.go": []byte(textstatSource)}}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Files: files, Plans: plans, Approver: &planApprover{}, Now: func() time.Time { return time.Unix(1, 0) }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModePlan); err != nil {
		t.Fatal(err)
	}
	var sent root.Text
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: &memoryStore{}, Continue: ContinueTurnUseCase{Store: &memoryStore{}, Ceremonies: d, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if sent == "" {
			sent = r.Messages[len(r.Messages)-1].Content
		}
		return assistant("planning"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "Add LineCount with its test", ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(sent), "Add LineCount with its test\n\n"+planBriefNote) {
		t.Fatalf("brief sent as %q", sent)
	}
	if record := BuildTurnRecord(s, 0, 1, "project:x", nil); record.Prompt != "Add LineCount with its test" {
		t.Fatalf("record prompt %q", record.Prompt)
	}
}

// On 20 October 2026 two citations of the 12-line test_calc.py put the same
// "--- test_calc.py:1-12 ---" excerpt in a task's pack twice. Regions of one
// file are merged where they overlap or touch; distant ones stay apart.
func TestPlanPacksQuoteEachCitedLineOnce(t *testing.T) {
	short := "import unittest\nfrom calc import add, sub\n\n\nclass TestCalc(unittest.TestCase):\n    def test_add(self):\n        self.assertEqual(add(2, 3), 5)\n\n    def test_sub(self):\n        self.assertEqual(sub(5, 3), 2)\n\n\n"
	var long strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&long, "line number %03d of the long file\n", i)
	}
	d := &CeremonyDriver{Files: &fakeFiles{files: map[string][]byte{"test_calc.py": []byte(short), "long.txt": []byte(long.String())}}}
	task := domain.PlanTask{ID: "add-sub-test", Context: []domain.Citation{
		{Path: "test_calc.py", Line: 7, Quote: "self.assertEqual(add(2, 3), 5)"},
		{Path: "long.txt", Line: 5, Quote: "line number 005"},
		{Path: "test_calc.py", Line: 10, Quote: "self.assertEqual(sub(5, 3), 2)"},
		{Path: "long.txt", Line: 90, Quote: "line number 090"},
		{Path: "long.txt", Line: 30, Quote: "line number 030"},
	}}
	excerpts, problems, err := d.citedExcerpts(context.Background(), task)
	if err != nil || len(problems) > 0 {
		t.Fatalf("problems %v err %v", problems, err)
	}
	var heads []string
	for _, excerpt := range excerpts {
		heads = append(heads, strings.SplitN(excerpt, "\n", 2)[0])
	}
	// long.txt 5 → 1-25 and 30 → 10-50 overlap; 90 → 70-100 stays apart.
	if got := strings.Join(heads, " | "); got != "--- test_calc.py:1-12 --- | --- long.txt:1-50 --- | --- long.txt:70-100 ---" {
		t.Fatalf("excerpts %s", got)
	}
	if !strings.HasSuffix(excerpts[1], "line number 050 of the long file") || strings.Count(excerpts[1], "line number 010") != 1 {
		t.Fatalf("merged excerpt:\n%s", excerpts[1])
	}
}

// On 20 October 2026 the person approved the card of a plan's check commands
// twice, and each time the console then refused the plan for a defect found
// without running anything: a test-first task with only tests in its scope,
// then a citation of the wrong line. Such a hand-back goes back to the
// planner without a card and runs nothing; a sound plan keeps its card.
func TestADefectivePlanGoesBackWithoutTheCommandsCard(t *testing.T) {
	stepDone, _ := domain.NewHostToolIdentity(domain.HostOperationStepDone)
	testsOnly := strings.Replace(goodTasks, `"scope":["lines.go","lines_test.go"]`, `"scope":["lines_test.go"],"test_first":true`, 1)
	wrongLine := strings.Replace(goodTasks, `"line":8`, `"line":7`, 1)
	for name, tasks := range map[string]string{"test-first over tests only": testsOnly, "wrong line": wrongLine} {
		d, _, _, _, s := planDriver(t)
		if err := s.BeginTurn("planifica", append(turnTools(), HostTools()...)); err != nil {
			t.Fatal(err)
		}
		handBack := root.ToolCall{ID: "decompose-1", Name: HostStepDoneName, Arguments: mustObject(t, planArgs(t, tasks))}
		if err := s.CompleteAssistant(assistant("", handBack)); err != nil {
			t.Fatal(err)
		}
		if approved, withheld := approvesCall(context.Background(), nil, d, s, stepDone, handBack.Arguments); !approved || !withheld {
			t.Fatalf("%s: the defective plan raised the commands card", name)
		}
		store := &memoryStore{}
		u := ResolveToolUseCase{Store: store, Continue: ContinueTurnUseCase{Store: store, Ceremonies: d}}
		if err := u.resolveOne(context.Background(), &s, "decompose-1", domain.DecisionAutoApprove, ignoreEvent); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if run, live := s.Ceremony(); !live || run.Step != "decompose" || run.Iteration != 2 || len(run.Plan.Defects) == 0 || len(run.Approved) != 0 {
			t.Fatalf("%s: the plan did not go back with its defects: %+v", name, run)
		}
		if runs := d.Checks.(*fakeChecks).runs; len(runs) != 0 {
			t.Fatalf("%s: a command ran without the person: %v", name, runs)
		}
	}
	d, _, _, _, s := planDriver(t)
	if approved, _ := approvesCall(context.Background(), nil, d, s, stepDone, mustObject(t, planArgs(t, goodTasks))); approved {
		t.Fatal("a sound plan's commands skipped the card")
	}
}

// A plan approved without a card because it looked defective must not run
// its commands if it passes when decompose reads the workspace again.
func TestAPlanApprovedAsDefectiveRunsNoCommandIfItPasses(t *testing.T) {
	d, _, _, _, s := planDriver(t)
	result, err := d.StepDone(withholdCommands(context.Background()), s, mustObject(t, planArgs(t, goodTasks)))
	if err != nil || !strings.Contains(string(result.Outcome.Content), "workspace changed") {
		t.Fatalf("outcome %s err %v", result.Outcome.Content, err)
	}
	if runs := d.Checks.(*fakeChecks).runs; len(runs) != 0 {
		t.Fatalf("a command ran without the person: %v", runs)
	}
}
