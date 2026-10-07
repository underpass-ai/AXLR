package application

import (
	"context"
	"encoding/json"
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
