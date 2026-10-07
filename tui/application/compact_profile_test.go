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

func TestCompactStepsAreSmallExactAndASubsetOfTheFullHandBack(t *testing.T) {
	var full map[string]any
	for _, tool := range HostTools() {
		if tool.Definition.Name == HostStepDoneName {
			if err := json.Unmarshal(tool.Definition.Parameters.Bytes(), &full); err != nil {
				t.Fatal(err)
			}
		}
	}
	fullProperties := full["properties"].(map[string]any)
	for name, step := range compactSteps {
		if len(step.instruction) >= 500 {
			t.Errorf("%s instruction is %d bytes", name, len(step.instruction))
		}
		if !strings.HasPrefix(step.instruction, "Example: axlr_step_done {") {
			t.Errorf("%s instruction does not lead with its example", name)
		}
		var schema map[string]any
		if err := json.Unmarshal([]byte(step.schema), &schema); err != nil {
			t.Fatalf("%s schema: %v", name, err)
		}
		properties := schema["properties"].(map[string]any)
		if len(properties) != len(step.fields) {
			t.Errorf("%s schema and fields differ", name)
		}
		for _, field := range step.fields {
			if properties[field] == nil || fullProperties[field] == nil {
				t.Errorf("%s field %s missing from its schema or from the full hand-back", name, field)
			}
		}
		example := step.instruction[len("Example: axlr_step_done "):]
		example = example[:strings.Index(example, "}. ")+1]
		done, ignored, err := decodeCompactStepDone(step, mustObject(t, example))
		if err != nil || len(ignored) != 0 {
			t.Errorf("%s example does not decode cleanly: %v %v", name, ignored, err)
		}
		_ = done
	}
}

func TestCompactDecodingForgivesTheUsualMistakes(t *testing.T) {
	brief := compactSteps["brief"]
	done, ignored, err := decodeCompactStepDone(brief, mustObject(t, `{"criteria":"c","scope":"s","check_command":"go test ./...","summary":"extra","notes":"x"}`))
	command, ok := done.command()
	if err != nil || !ok || command.Program != "go" || strings.Join(command.Args, " ") != "test ./..." || strings.Join(ignored, ",") != "notes,summary" {
		t.Fatalf("decoded %+v %v ignored=%v err=%v", done, command, ignored, err)
	}
	done, _, err = decodeCompactStepDone(brief, mustObject(t, `{"criteria":"c","scope":"s","check_command":{"program":"go","args":"test -run X ./..."}}`))
	if command, _ := done.command(); err != nil || strings.Join(command.Args, "|") != "test|-run|X|./..." {
		t.Fatalf("args string: %+v %v", command, err)
	}
	for _, shell := range []string{`"go test ./... | tee log"`, `"cd x && go test"`, `{"program":"sh","args":"-c 'go test'"}`} {
		if _, _, err := decodeCompactStepDone(brief, mustObject(t, `{"criteria":"c","scope":"s","check_command":`+shell+`}`)); err == nil {
			t.Errorf("shell syntax accepted: %s", shell)
		}
	}
}

func compactDeliverySession(t *testing.T, step string) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDelivery); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("fix WordCount", append(turnTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_delivery", Version: "2.0", Instance: "i", Step: step, Iteration: 1, Compact: true}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCompactStepOffersOnlyWhatTheStepNeeds(t *testing.T) {
	local := []domain.AvailableTool{}
	for _, name := range []string{"local_read", "local_write", "local_edit", "local_exec"} {
		id, _ := domain.NewLocalToolIdentity(strings.TrimPrefix(name, "local_"))
		schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
		local = append(local, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName(name), Parameters: schema}})
	}
	names := func(step string) (string, string) {
		s := compactDeliverySession(t, step)
		tools := SessionTools(s, append(local, HostTools()...))
		var out []string
		schema := ""
		for _, tool := range tools {
			out = append(out, string(tool.Name))
			if tool.Name == HostStepDoneName {
				schema = string(tool.Parameters.Bytes())
			}
		}
		return strings.Join(out, ","), schema
	}
	brief, briefSchema := names("brief")
	if brief != "axlr_history,axlr_step_done,local_exec,local_read" || briefSchema != compactSteps["brief"].schema {
		t.Fatalf("brief tools = %s schema=%s", brief, briefSchema)
	}
	if build, _ := names("build"); build != "axlr_history,axlr_step_done,local_edit,local_exec,local_read,local_write" {
		t.Fatalf("build tools = %s", build)
	}
	s := compactDeliverySession(t, "brief")
	guidance := string(modelHostGuidance(&s).Content)
	if len(guidance) > 2048 || strings.Contains(guidance, "axlr_call_tool") || strings.Contains(guidance, "Self-repair") || !strings.Contains(guidance, "Example: axlr_step_done") {
		t.Fatalf("compact guidance (%d bytes): %s", len(guidance), guidance)
	}
}

func TestCompactRefusesHiddenToolsAndRepeatedCalls(t *testing.T) {
	s := compactDeliverySession(t, "brief")
	write := root.ToolCall{ID: "w1", Name: "local_write", Arguments: mustObject(t, `{"path":"a"}`)}
	bridge := root.ToolCall{ID: "b1", Name: HostCallToolName, Arguments: mustObject(t, `{"name":"x","arguments":{}}`)}
	read := func(id root.ToolCallID) root.ToolCall {
		return root.ToolCall{ID: id, Name: "local_read", Arguments: mustObject(t, `{"path":"textstat.go"}`)}
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{write, bridge, read("r1"), read("r2")}}}); err != nil {
		t.Fatal(err)
	}
	pending := s.Pending()
	if err := compactRefusal(s, pending[0]); err == nil || !strings.Contains(err.Error(), "only reads") {
		t.Fatalf("write in a read-only step: %v", err)
	}
	if err := compactRefusal(s, pending[1]); err == nil {
		t.Fatal("hidden host tool accepted")
	}
	if err := compactRefusal(s, pending[2]); err != nil {
		t.Fatalf("first read refused: %v", err)
	}
	if err := compactRefusal(s, pending[3]); err == nil || !strings.Contains(err.Error(), "same call as before") {
		t.Fatalf("repeated read: %v", err)
	}
}

func TestCompactStepDoneWithAStringCommandStillNeedsApproval(t *testing.T) {
	s := compactDeliverySession(t, "brief")
	if !stepDoneNeedsApproval(s, mustObject(t, `{"criteria":"c","scope":"s","check_command":"go test ./..."}`)) {
		t.Fatal("a recovered command skipped the approval card")
	}
}

func TestCompactDeliveryKeepsALedgerAndStartsEachStepFromIt(t *testing.T) {
	engine, checks := &fakeEngine{}, &fakeChecks{exits: []int{1, 0}}
	d := &CeremonyDriver{Engine: engine, Checks: checks, Now: func() time.Time { return time.Unix(1, 0) }, Compact: func(root.ModelID) bool { return true }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDelivery); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin(context.Background(), &s, "fix WordCount"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("fix WordCount", append(turnTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	if run, _ := s.Ceremony(); !run.Compact || run.StepCallLimit() != domain.CompactStepCalls {
		t.Fatalf("begin: %+v", run)
	}
	// The model reads, then hands back the brief with a string command.
	handBack := root.ToolCall{ID: "s1", Name: HostStepDoneName, Arguments: mustObject(t, `{"criteria":"blank is 0","scope":"textstat.go","check_command":"go test ./...","note":"x"}`)}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{read1(t), handBack}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("r1", domain.DecisionAutoApprove, domain.ToolOutcome{Content: "package textstat"}); err != nil {
		t.Fatal(err)
	}
	result, err := d.StepDone(context.Background(), s, handBack.Arguments)
	if err != nil || !result.Accepted {
		t.Fatalf("brief: %+v %v", result, err)
	}
	if !strings.Contains(string(result.Outcome.Content), `"ignored_fields":["note"]`) {
		t.Fatalf("ignored field not named: %s", result.Outcome.Content)
	}
	if err := s.RecordToolOutcome("s1", domain.DecisionAutoApprove, result.Outcome); err != nil {
		t.Fatal(err)
	}
	run := *result.Run
	run.StepCall = "s1"
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	if len(run.Ledger) != 1 || !strings.Contains(run.Ledger[0].Text, "brief #1") || !strings.Contains(run.Ledger[0].Text, "check go test ./... → exit 1") {
		t.Fatalf("ledger = %+v", run.Ledger)
	}
	messages, origin := ledgerProjection(s, s.Messages())
	if origin == nil || messages[0].Role != root.RoleUser || !strings.Contains(string(messages[0].Content), compactLedgerHeading) || messages[1].ToolCalls[1].ID != "s1" {
		t.Fatalf("projection = %+v origin=%v", messages, origin)
	}
	if origin[0] != 0 || origin[1] != 1 {
		t.Fatalf("origin = %v", origin)
	}
}

func read1(t *testing.T) root.ToolCall {
	return root.ToolCall{ID: "r1", Name: "read", Arguments: mustObject(t, `{"path":"textstat.go"}`)}
}

func TestCompactBudgetIsTheSmallerOfTheTwo(t *testing.T) {
	budget := domain.ContextBudgetForWindow(65536).Smaller(domain.CompactContextBudget())
	if budget.MaximumBytes() != 80<<10 || budget.ToolResultBytes() != 8<<10 || budget.CheckpointBytes() != 4<<10 {
		t.Fatalf("budget = %d/%d/%d/%d", budget.MaximumBytes(), budget.LowWaterBytes(), budget.ToolResultBytes(), budget.CheckpointBytes())
	}
	if small := domain.ContextBudgetForWindow(16384).Smaller(domain.CompactContextBudget()); small.MaximumBytes() != 36864 {
		t.Fatalf("a 16K window must shrink further: %d", small.MaximumBytes())
	}
}

func TestMalformedExecCallsAreRepaired(t *testing.T) {
	for raw, want := range map[string]string{
		`{"program":"go test","args":["«-count=1»","«./...»"]}`: `{"args":["test","-count=1","./..."],"program":"go"}`,
		"{\"program\":\"go\",\"args\":[\"test\",\"`.`\"]}":      `{"args":["test","."],"program":"go"}`,
		`{"program":"«go test»","args":["./..."]}`:              `{"args":["test","./..."],"program":"go"}`,
		`{"program":"ls","args":["-R","<|\"|>.<|\"|>"]}`:        `{"args":["-R","."],"program":"ls"}`,
	} {
		got, changed := normalizeExec(mustObject(t, raw))
		if !changed || string(got.Bytes()) != want {
			t.Errorf("%s → %s (changed=%v), want %s", raw, got.Bytes(), changed, want)
		}
	}
	for _, untouched := range []string{`{"program":"go","args":["test","./..."]}`, `{"program":"sh -c 'x | y'"}`, `{"program":"go","args":["test"],"timeout_ms":5}`} {
		if got, changed := normalizeExec(mustObject(t, untouched)); changed {
			t.Errorf("%s changed to %s", untouched, got.Bytes())
		}
	}
	done, _, err := decodeCompactStepDone(compactSteps["brief"], mustObject(t, `{"criteria":"c","scope":"s","check_command":{"program":"go test","args":["«./...»"]}}`))
	if command, _ := done.command(); err != nil || command.Program != "go" || strings.Join(command.Args, " ") != "test ./..." {
		t.Fatalf("check_command: %+v %v", command, err)
	}
}

func TestRedWritesOnlyTests(t *testing.T) {
	s := compactDeliverySession(t, "red")
	run, _ := s.Ceremony()
	run.Definition = "axlr_task"
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	code := root.ToolCall{ID: "c1", Name: "local_write", Arguments: mustObject(t, `{"path":"textstat.go","content":"x"}`)}
	test := root.ToolCall{ID: "c2", Name: "local_edit", Arguments: mustObject(t, `{"path":"pkg/textstat_test.go","old_text":"a","new_text":"b"}`)}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{code, test}}}); err != nil {
		t.Fatal(err)
	}
	pending := s.Pending()
	if err := compactRefusal(s, pending[0]); err == nil || !strings.Contains(err.Error(), "only the failing test") {
		t.Fatalf("code write in red: %v", err)
	}
	if err := compactRefusal(s, pending[1]); err != nil {
		t.Fatalf("test edit in red refused: %v", err)
	}
}
