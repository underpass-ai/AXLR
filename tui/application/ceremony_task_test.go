package application

import (
	"context"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// taskChecks answers git status from the files the test changed and the unit
// check from a queue of exit codes.
type taskChecks struct {
	status []string
	exits  []int
	runs   []string
}

func (c *taskChecks) Run(_ context.Context, command domain.CheckCommand) (CheckResult, error) {
	line := command.Program + " " + strings.Join(command.Args, " ")
	c.runs = append(c.runs, line)
	if command.Program == "git" {
		if command.Args[0] == "status" {
			return CheckResult{Ran: true, Output: strings.Join(c.status, "\n")}, nil
		}
		return CheckResult{Ran: true, Output: "rev123\n"}, nil
	}
	exit := 0
	if len(c.exits) > 0 {
		exit, c.exits = c.exits[0], c.exits[1:]
	}
	return CheckResult{Ran: true, ExitCode: exit, Output: "FAIL lines_test.go: undefined: LineCount"}, nil
}

func taskSetup(t *testing.T, testFirst bool) (*CeremonyDriver, *fakeFiles, *taskChecks, *memoryPlans, domain.Session, domain.PlanRecord) {
	t.Helper()
	files := &fakeFiles{files: map[string][]byte{"textstat.go": []byte(textstatSource), "notes.md": []byte("x")}}
	checks := &taskChecks{status: []string{"?? notes.md"}, exits: []int{1}}
	plans := &memoryPlans{}
	task := domain.PlanTask{ID: "linecount", Goal: "Add LineCount.", Scope: []string{"lines.go", "lines_test.go"}, Protect: []string{"textstat.go"}, UnitCheck: domain.CheckCommand{Program: "go", Args: []string{"test", "./..."}}, TestFirst: testFirst, Wave: 1, Pack: "pack", Status: domain.TaskRunning}
	plan := domain.PlanRecord{ID: "p-1", Session: "0123456789abcdef0123456789abcdef", Workspace: "/w", Worker: "local/gemma", Status: domain.PlanRunning, Waves: 1, Tasks: []domain.PlanTask{task}}
	_ = plans.Save(context.Background(), plan)
	d := &CeremonyDriver{Engine: &fakeEngine{}, Checks: checks, Files: files, Plans: plans, Now: func() time.Time { return time.Unix(1, 0) }, Compact: func(root.ModelID) bool { return true }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeTask); err != nil {
		t.Fatal(err)
	}
	if err := d.BeginTask(context.Background(), &s, plan, task); err != nil {
		t.Fatal(err)
	}
	return d, files, checks, plans, s, plan
}

func TestATaskRunsRedGreenAndHandsBack(t *testing.T) {
	d, files, checks, plans, s, _ := taskSetup(t, true)
	run, _ := s.Ceremony()
	if run.Step != "red" || !run.Compact || run.Task.Start["notes.md"] == "" || !run.Task.Git {
		t.Fatalf("begin: step=%s %+v", run.Step, run.Task)
	}
	// The worker writes the failing test.
	files.files["lines_test.go"] = []byte("package textstat\nfunc TestLineCount(t *testing.T) { LineCount(\"\") }\n")
	checks.status = append(checks.status, "?? lines_test.go")
	checks.exits = []int{1}
	if r := step(t, d, &s, `{"test_files":["lines_test.go"],"expected":"undefined: LineCount"}`); r["next_step"] != "green" {
		t.Fatalf("red: %v", r)
	}
	// Changing a frozen test is refused without spending the attempt.
	files.files["lines_test.go"] = []byte("package textstat\n")
	if r := step(t, d, &s, `{"summary":"s","summary_en":"s"}`); !strings.Contains(r["error"].(string), "changed after red") {
		t.Fatalf("frozen test: %v", r)
	}
	files.files["lines_test.go"] = []byte("package textstat\nfunc TestLineCount(t *testing.T) { LineCount(\"\") }\n")
	// A change outside the scope is refused with the list.
	files.files["README.md"] = []byte("changed")
	checks.status = append(checks.status, " M README.md")
	if r := step(t, d, &s, `{"summary":"s","summary_en":"s"}`); !strings.Contains(r["error"].(string), "outside the task's scope: README.md") {
		t.Fatalf("scope: %v", r)
	}
	delete(files.files, "README.md")
	checks.status = checks.status[:len(checks.status)-1]
	files.files["lines.go"] = []byte("package textstat\nfunc LineCount(s string) int { return 0 }\n")
	checks.status = append(checks.status, "?? lines.go")
	checks.exits = []int{0}
	r := step(t, d, &s, `{"summary":"Added LineCount","summary_en":"LineCount counts lines.","notes":[{"to":"all","text":"LineCount ignores a final newline"}],"questions":["Count CRLF?"]}`)
	if r["ceremony"] != "DONE" {
		t.Fatalf("green to done: %v", r)
	}
	task := plans.records[0].Tasks[0]
	if task.Status != domain.TaskDone || task.Handback == nil || task.Handback.Notes[0].Text != "LineCount ignores a final newline" || strings.Join(task.Handback.Changed, ",") != "lines.go,lines_test.go" || task.Handback.Revision != "rev123" {
		t.Fatalf("hand-back: %+v %+v", task, task.Handback)
	}
}

func TestTaskEndsAtDoneWithoutAnotherModelRequest(t *testing.T) {
	d, files, checks, _, s, _ := taskSetup(t, true)
	files.files["lines_test.go"] = []byte("package textstat\nfunc TestLineCount(t *testing.T) { LineCount(\"\") }\n")
	checks.status = append(checks.status, "?? lines_test.go")
	checks.exits = []int{1}
	if r := step(t, d, &s, `{"test_files":["lines_test.go"],"expected":"undefined: LineCount"}`); r["next_step"] != "green" {
		t.Fatalf("red: %v", r)
	}
	files.files["lines.go"] = []byte("package textstat\nfunc LineCount(s string) int { return 0 }\n")
	checks.status = append(checks.status, "?? lines.go")
	checks.exits = []int{0}
	if err := s.BeginTurn("go", append(turnTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	// The model hands the green step back through axlr_step_done as its last call.
	handBack := root.ToolCall{ID: "done", Name: HostStepDoneName, Arguments: mustObject(t, `{"summary":"Added LineCount","summary_en":"LineCount counts lines."}`)}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{handBack}}}); err != nil {
		t.Fatal(err)
	}
	modelRequests := 0
	models := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		modelRequests++
		return assistant("done"), nil
	})
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Continue: ContinueTurnUseCase{Store: store, Ceremonies: d, Models: models}}
	if err := u.Execute(context.Background(), &s, "done", domain.DecisionApprove, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if _, live := s.Ceremony(); live || s.Mode() != domain.ModeNormal {
		t.Fatal("task ceremony still live after DONE")
	}
	if modelRequests != 0 {
		t.Fatalf("model requests after DONE = %d, want 0", modelRequests)
	}
}

func TestATaskThatCannotBeTestedEndsBlocked(t *testing.T) {
	d, _, _, plans, s, _ := taskSetup(t, true)
	if r := step(t, d, &s, `{"untestable":true,"observed":"the behaviour needs a network"}`); r["ceremony"] != "BLOCKED" {
		t.Fatalf("untestable: %v", r)
	}
	if task := plans.records[0].Tasks[0]; task.Status != domain.TaskBlocked || !strings.Contains(task.Reason, "needs a network") {
		t.Fatalf("blocked task: %+v", task)
	}
}

func TestTaskPromptCarriesNotesForTheTask(t *testing.T) {
	plan := domain.PlanRecord{ID: "p", Tasks: []domain.PlanTask{
		{ID: "a", Handback: &domain.TaskHandback{Notes: []domain.TaskNote{{From: "a", To: "b", Text: "use Fields"}, {From: "a", To: "c", Text: "not for b"}, {From: "a", To: "all", Text: "for all"}}}},
		{ID: "b", Pack: "PACK\n"},
	}}
	prompt := string(TaskPrompt(plan, plan.Tasks[1]))
	if !strings.HasPrefix(prompt, "PACK") || !strings.Contains(prompt, "use Fields") || !strings.Contains(prompt, "for all") || strings.Contains(prompt, "not for b") {
		t.Fatalf("prompt = %q", prompt)
	}
}
