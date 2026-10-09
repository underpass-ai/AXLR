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

// forgedMemory is an in-memory ForgedToolsPort.
type forgedMemory struct {
	tools     []ForgedTool
	verifyErr error
}

func (f *forgedMemory) List(context.Context, string) ([]ForgedTool, error) { return f.tools, nil }
func (f *forgedMemory) Forge(_ context.Context, _ string, tool ForgedTool, files []ForgedFile) (ForgedTool, bool, error) {
	tool.Files = map[string]string{}
	for _, file := range files {
		tool.Files[file.Path] = "digest"
	}
	for i, existing := range f.tools {
		if existing.Name == tool.Name {
			f.tools[i] = tool
			return tool, true, nil
		}
	}
	f.tools = append(f.tools, tool)
	return tool, false, nil
}
func (f *forgedMemory) Verify(context.Context, string, ForgedTool) error { return f.verifyErr }

func hostIdentity(t *testing.T, operation string) domain.ToolIdentity {
	t.Helper()
	id, err := domain.NewHostToolIdentity(operation)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func object(t *testing.T, text string) root.JSONValue {
	t.Helper()
	value, err := root.NewJSONObject([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

const forgeArguments = `{"name":"word_count","description":"Count the words of text.","input_schema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]},"program":"python3","args":[".axlr/tools/word_count/main.py"],"files":[{"path":"main.py","content":"import json,sys\nprint(len(json.load(sys.stdin)['text'].split()))\n"}]}`

// Forging writes code into the workspace and running a forged tool runs a
// program: the modes and the approval policy treat them as local_write and
// local_exec.
func TestForgedToolsAreJudgedAndApprovedAsWriteAndExec(t *testing.T) {
	forge, run := hostIdentity(t, domain.HostOperationForgeTool), hostIdentity(t, domain.HostOperationRunTool)
	for _, mode := range []domain.WorkMode{domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		if verdict, _ := mode.Judge(forge, object(t, forgeArguments)); verdict != domain.VerdictDeny {
			t.Fatalf("%s mode let the model forge a tool", mode)
		}
		if verdict, _ := mode.Judge(run, object(t, `{"name":"word_count","arguments":{}}`)); verdict != domain.VerdictAsk {
			t.Fatalf("%s mode ran a forged tool without the person", mode)
		}
	}
	if verdict, _ := domain.ModeNormal.Judge(forge, object(t, forgeArguments)); verdict != domain.VerdictAllow {
		t.Fatal("normal mode refused forging")
	}
	write, exec := localIdentity("write"), localIdentity("exec")
	onlyWrite := approvalFunc(func(id domain.ToolIdentity) bool { return id == write })
	onlyExec := approvalFunc(func(id domain.ToolIdentity) bool { return id == exec })
	if !automaticallyApproves(onlyWrite, forge) || automaticallyApproves(onlyExec, forge) || automaticallyApproves(nil, forge) {
		t.Fatal("axlr_forge_tool is not approved as local_write")
	}
	if !automaticallyApproves(onlyExec, run) || automaticallyApproves(onlyWrite, run) || automaticallyApproves(nil, run) {
		t.Fatal("axlr_run_tool is not approved as local_exec")
	}
}

func TestForgedToolsAreOfferedInOrdinarySessions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         domain.WorkMode
		port         bool
		forge, run   bool
		changesTools bool
	}{
		{"normal", domain.ModeNormal, true, true, true, true},
		{"turned off", domain.ModeNormal, false, false, false, false},
		{"review runs but does not forge", domain.ModeReview, true, false, true, true},
		{"plan keeps its own tools", domain.ModePlan, true, false, false, false},
		{"repair keeps its own tools", domain.ModeRepair, true, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := turnSession(t)
			if tc.mode != domain.ModeNormal {
				if err := s.SetMode(tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.BeginTurn("count the words", turnTools()); err != nil {
				t.Fatal(err)
			}
			var request root.CompletionRequest
			u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
				request = req
				return assistant("ok"), nil
			})}
			if tc.port {
				u.Forge = &forgedMemory{}
			}
			if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if requestHasTool(request, HostForgeToolName) != tc.forge || requestHasTool(request, HostRunToolName) != tc.run {
				t.Fatalf("forge offered %v, run offered %v", requestHasTool(request, HostForgeToolName), requestHasTool(request, HostRunToolName))
			}
		})
	}
}

// A forged tool changes neither the request's tools nor its system prompt,
// so the prompt cache survives it.
func TestForgingKeepsTheRequestPrefix(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	if err := s.BeginTurn("count the words", turnTools()); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	u := ContinueTurnUseCase{Store: &memoryStore{}, Forge: forged, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		requests = append(requests, req)
		return assistant("ok"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if _, err := (HostToolUseCase{Forge: forged}).Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("again", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	first, second := requests[0], requests[1]
	encodedFirst, _ := json.Marshal(first.Tools)
	encodedSecond, _ := json.Marshal(second.Tools)
	if string(encodedFirst) != string(encodedSecond) || first.Messages[0].Content != second.Messages[0].Content {
		t.Fatal("forging a tool changed the request prefix")
	}
}

func TestForgeRefusesUnsafeOrIncompleteTools(t *testing.T) {
	s := turnSession(t)
	host := HostToolUseCase{Forge: &forgedMemory{}}
	forge := hostIdentity(t, domain.HostOperationForgeTool)
	for name, tc := range map[string]struct {
		edit func(map[string]any)
		want string
	}{
		"escaping path": {func(a map[string]any) { a["files"] = []any{map[string]any{"path": "../../evil.sh", "content": "x"}} }, "relative to the tool's directory"},
		"absolute path": {func(a map[string]any) { a["files"] = []any{map[string]any{"path": "/etc/passwd", "content": "x"}} }, "relative to the tool's directory"},
		"program path":  {func(a map[string]any) { a["program"] = "/bin/sh" }, "not a path"},
		"no own file":   {func(a map[string]any) { a["args"] = []any{"-c", "rm -rf ~"} }, "must name one of the tool's files"},
		"bad name":      {func(a map[string]any) { a["name"] = "Count Words" }, "lowercase"},
		"schema type":   {func(a map[string]any) { a["input_schema"] = map[string]any{"type": "string"} }, "must have"},
		"unknown field": {func(a map[string]any) { a["env"] = map[string]any{} }, "must match its schema"},
		"too large": {func(a map[string]any) {
			a["files"] = []any{map[string]any{"path": "main.py", "content": strings.Repeat("x", maxForgedFileBytes+1)}}
		}, "exceeds"},
		"duplicate files": {func(a map[string]any) {
			a["files"] = []any{map[string]any{"path": "main.py", "content": "x"}, map[string]any{"path": "./main.py", "content": "y"}}
		}, "twice"},
	} {
		t.Run(name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal([]byte(forgeArguments), &args); err != nil {
				t.Fatal(err)
			}
			tc.edit(args)
			encoded, _ := json.Marshal(args)
			outcome, err := host.Execute(context.Background(), s, forge, object(t, string(encoded)))
			if err != nil || !outcome.IsError || !strings.Contains(string(outcome.Content), tc.want) {
				t.Fatalf("outcome = %s, %v; want %q", outcome.Content, err, tc.want)
			}
		})
	}
}

func TestRunToolExecutesTheForgedCommandWithItsArgumentsOnStdin(t *testing.T) {
	s := turnSession(t)
	forged := &forgedMemory{}
	var ran domain.ToolIdentity
	var command struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
		Stdin   string   `json:"stdin"`
	}
	tools := toolFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		ran = id
		if err := json.Unmarshal(args.Bytes(), &command); err != nil {
			t.Fatal(err)
		}
		return domain.ToolOutcome{Content: `{"status":"completed"}`}, nil
	})
	validated := 0
	host := HostToolUseCase{Forge: forged, Tools: tools, Validation: argumentValidationFunc(func(tool root.ToolDefinition, args root.JSONValue) error {
		validated++
		if !strings.Contains(string(args.Bytes()), `"text"`) {
			return errors.New("missing text")
		}
		return nil
	})}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	run := hostIdentity(t, domain.HostOperationRunTool)
	outcome, err := host.Execute(context.Background(), s, run, object(t, `{"name":"word_count","arguments":{"text":"one two"}}`))
	if err != nil || outcome.IsError {
		t.Fatalf("run = %s, %v", outcome.Content, err)
	}
	if ran != localIdentity("exec") || command.Program != "python3" || len(command.Args) != 1 || command.Args[0] != ".axlr/tools/word_count/main.py" || command.Stdin != `{"text":"one two"}` {
		t.Fatalf("ran %+v with %+v", ran, command)
	}
	if outcome, _ := host.Execute(context.Background(), s, run, object(t, `{"name":"word_count","arguments":{"other":1}}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "input_schema") {
		t.Fatalf("invalid arguments ran: %s", outcome.Content)
	}
	if outcome, _ := host.Execute(context.Background(), s, run, object(t, `{"name":"missing","arguments":{}}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "unknown forged tool") {
		t.Fatalf("unknown tool: %s", outcome.Content)
	}
	forged.verifyErr = errors.New("forged tool \"word_count\" changed since it was forged")
	if outcome, _ := host.Execute(context.Background(), s, run, object(t, `{"name":"word_count","arguments":{"text":"x"}}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "changed since") {
		t.Fatalf("changed tool ran: %s", outcome.Content)
	}
	if validated != 2 {
		t.Fatalf("validated %d times", validated)
	}
}

func TestAxlrToolsFindsForgedTools(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	tools := hostIdentity(t, domain.HostOperationTools)
	search, _ := host.Execute(context.Background(), s, tools, object(t, `{"query":"words"}`))
	if !strings.Contains(string(search.Content), `"name":"word_count"`) || !strings.Contains(string(search.Content), `"call_with":"axlr_run_tool"`) {
		t.Fatalf("search: %s", search.Content)
	}
	exact, _ := host.Execute(context.Background(), s, tools, object(t, `{"name":"word_count"}`))
	if exact.IsError || !strings.Contains(string(exact.Content), `"forged":true`) || !strings.Contains(string(exact.Content), "main.py") {
		t.Fatalf("exact: %s", exact.Content)
	}
}

// A forged tool's failure belongs to that tool: fixing AXLR would not change it.
func TestAForgedToolFailureIsNotEvidenceOfADefectOfAXLR(t *testing.T) {
	run := RunTool()
	_, refusal := classifyCall([]domain.AvailableTool{run}, domain.PendingTool{Call: root.ToolCall{ID: "r1", Name: HostRunToolName}, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Content: `{"error":"boom"}`, IsError: true}})
	if !strings.Contains(refusal, "axlr_forge_tool") {
		t.Fatalf("refusal = %q", refusal)
	}
}
