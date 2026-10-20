package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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
func (f *forgedMemory) Remove(_ context.Context, _ string, name string) (bool, error) {
	for i, existing := range f.tools {
		if existing.Name == name {
			f.tools = append(f.tools[:i], f.tools[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

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
	remove := hostIdentity(t, domain.HostOperationRemoveTool)
	for _, mode := range []domain.WorkMode{domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		if verdict, _ := mode.Judge(forge, object(t, forgeArguments)); verdict != domain.VerdictDeny {
			t.Fatalf("%s mode let the model forge a tool", mode)
		}
		if verdict, _ := mode.Judge(remove, object(t, `{"name":"word_count"}`)); verdict != domain.VerdictDeny {
			t.Fatalf("%s mode let the model remove a tool", mode)
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
	if !automaticallyApproves(onlyWrite, remove) || automaticallyApproves(onlyExec, remove) || automaticallyApproves(nil, remove) {
		t.Fatal("axlr_remove_tool is not approved as local_write")
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
			// Removing is a write, offered where forging is.
			if requestHasTool(request, HostRemoveToolName) != tc.forge {
				t.Fatalf("remove offered %v", requestHasTool(request, HostRemoveToolName))
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
		"escaping path": {func(a map[string]any) { a["files"] = []any{map[string]any{"path": "../../evil.sh", "content": "x"}} }, "inside the tool's directory"},
		"absolute path": {func(a map[string]any) { a["files"] = []any{map[string]any{"path": "/etc/passwd", "content": "x"}} }, "inside the tool's directory"},
		"other tool's path": {func(a map[string]any) {
			a["files"] = []any{map[string]any{"path": ".axlr/tools/other/main.py", "content": "x"}}
		}, "inside the tool's directory"},
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

// The approved call goes through the resolver, which hands the continuation's
// forged tools, the local runtime and the validator to the host tools.
func TestAnApprovedForgedRunReachesTheRuntimeThroughTheResolver(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	if _, err := (HostToolUseCase{Forge: forged}).Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("count the words", append(turnTools(), RunTool())); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", root.ToolCall{ID: "call-1", Name: HostRunToolName, Arguments: object(t, `{"name":"word_count","arguments":{"text":"one two"},"why":"count the words of the sample"}`)})); err != nil {
		t.Fatal(err)
	}
	var ran domain.ToolIdentity
	var stdin string
	tools := toolFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		var command struct{ Stdin string }
		_ = json.Unmarshal(args.Bytes(), &command)
		ran, stdin = id, command.Stdin
		return domain.ToolOutcome{Content: `{"status":"completed","output":{"exit_code":0,"stdout":"2\n"}}`}, nil
	})
	validated := false
	store := &memoryStore{}
	var sent root.CompletionRequest
	u := ResolveToolUseCase{Store: store, Tools: tools, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { validated = true; return nil }),
		Continue: ContinueTurnUseCase{Store: store, Forge: forged, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			sent = req
			return assistant("two words"), nil
		})}}
	if err := u.Execute(context.Background(), &s, "call-1", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if ran != localIdentity("exec") || stdin != `{"text":"one two"}` || !validated {
		t.Fatalf("ran %+v with stdin %q, validated %v", ran, stdin, validated)
	}
	if result := sent.Messages[len(sent.Messages)-1]; result.Role != root.RoleTool || !strings.Contains(string(result.Content), `"stdout":"2\n"`) {
		t.Fatalf("result = %s", result.Content)
	}
}

// Haiku named the file by its workspace path on 9 Oct 2026, three times in a
// row; that path means the same file.
func TestForgeAcceptsAFileNamedByItsWorkspacePath(t *testing.T) {
	forged := &forgedMemory{}
	var args map[string]any
	if err := json.Unmarshal([]byte(forgeArguments), &args); err != nil {
		t.Fatal(err)
	}
	args["files"] = []any{map[string]any{"path": ".axlr/tools/word_count/main.py", "content": "print(1)"}}
	encoded, _ := json.Marshal(args)
	outcome, err := (HostToolUseCase{Forge: forged}).Execute(context.Background(), turnSession(t), hostIdentity(t, domain.HostOperationForgeTool), object(t, string(encoded)))
	if err != nil || outcome.IsError || len(forged.tools) != 1 || forged.tools[0].Files["main.py"] == "" {
		t.Fatalf("outcome = %s, %v, tools %+v", outcome.Content, err, forged.tools)
	}
}

func TestAxlrToolsFallsBackToPartialMatches(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	tools := hostIdentity(t, domain.HostOperationTools)
	search, _ := host.Execute(context.Background(), s, tools, object(t, `{"query":"sales count words report"}`))
	if !strings.Contains(string(search.Content), `"name":"word_count"`) || !strings.Contains(string(search.Content), `"partial":true`) {
		t.Fatalf("search: %s", search.Content)
	}
	none, _ := host.Execute(context.Background(), s, tools, object(t, `{"query":"invoices"}`))
	if strings.Contains(string(none.Content), "word_count") || strings.Contains(string(none.Content), "partial") {
		t.Fatalf("search: %s", none.Content)
	}
}

// axlr_remove_tool uninstalls a forged tool: axlr_tools no longer lists it
// and axlr_run_tool no longer knows it.
func TestRemovingAForgedTool(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	remove := hostIdentity(t, domain.HostOperationRemoveTool)
	outcome, err := host.Execute(context.Background(), s, remove, object(t, `{"name":"word_count"}`))
	if err != nil || outcome.IsError || !strings.Contains(string(outcome.Content), `"removed":"word_count"`) {
		t.Fatalf("remove = %s, %v", outcome.Content, err)
	}
	if len(forged.tools) != 0 {
		t.Fatalf("still registered: %+v", forged.tools)
	}
	for _, args := range []string{`{"name":"word_count"}`, `{"name":"../etc"}`, `{"other":1}`} {
		if outcome, _ := host.Execute(context.Background(), s, remove, object(t, args)); !outcome.IsError {
			t.Fatalf("remove %s succeeded: %s", args, outcome.Content)
		}
	}
	search, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationTools), object(t, `{"query":"words"}`))
	if strings.Contains(string(search.Content), "word_count") {
		t.Fatalf("removed tool listed: %s", search.Content)
	}
	if !strings.Contains(string(RemoveTool().Definition.Description), "Uninstall") || !strings.Contains(string(ForgeTool().Definition.Description), string(HostRemoveToolName)) {
		t.Fatal("the descriptions do not point the model at axlr_remove_tool")
	}
}

// The forged-tool checks that change nothing run before the card. On 16
// October 2026 four invalid forges (a path outside the tool's directory, a
// -c command, bash /etc/passwd, a name with a space), a run of a tool edited
// outside the console and a removal of an unregistered directory each raised
// a card, the person approved it, and the console then refused the call.
func TestForgedCallsTheConsoleWouldRefuseAreRefusedBeforeTheCard(t *testing.T) {
	registered := ForgedTool{Name: "csv_stats", Description: "stats", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`), Program: "python3", Args: []string{".axlr/tools/csv_stats/main.py"}}
	for name, test := range map[string]struct {
		tool      domain.AvailableTool
		arguments string
		verifyErr error
		invalid   bool
		want      string
	}{
		"path outside the tool": {tool: ForgeTool(), arguments: `{"name":"evil","description":"qa","input_schema":{"type":"object"},"program":"python3","args":[".axlr/tools/evil/main.py"],"files":[{"path":"../../escape.py","content":"x"},{"path":"main.py","content":"x"}]}`, want: `axlr_forge_tool: file path "../../escape.py" must be inside the tool's directory`},
		"args name no file":     {tool: ForgeTool(), arguments: `{"name":"evil","description":"qa","input_schema":{"type":"object"},"program":"bash","args":["/etc/passwd"],"files":[{"path":"run.sh","content":"x"}]}`, want: "args must name one of the tool's files"},
		"bad name":              {tool: ForgeTool(), arguments: `{"name":"Bad Name!","description":"qa","input_schema":{"type":"object"},"program":"python3","args":["main.py"],"files":[{"path":"main.py","content":"x"}]}`, want: "name must be 2-48 lowercase"},
		"valid forge":           {tool: ForgeTool(), arguments: forgeArguments},
		"changed tool":          {tool: RunTool(), arguments: `{"name":"csv_stats","arguments":{"path":"a.csv"},"why":"stats of the sample"}`, verifyErr: errors.New(`forged tool "csv_stats" changed since it was forged (main.py)`), want: "axlr_run_tool: forged tool \"csv_stats\" changed since it was forged"},
		"unknown tool":          {tool: RunTool(), arguments: `{"name":"missing","arguments":{},"why":"run the missing tool"}`, want: `unknown forged tool "missing"`},
		"arguments off schema":  {tool: RunTool(), arguments: `{"name":"csv_stats","arguments":{"path":5},"why":"stats of the sample"}`, invalid: true, want: "arguments do not match csv_stats's input_schema"},
		"valid run":             {tool: RunTool(), arguments: `{"name":"csv_stats","arguments":{"path":"a.csv"},"why":"stats of the sample"}`},
		"unregistered removal":  {tool: RemoveTool(), arguments: `{"name":"stray"}`, want: `axlr_remove_tool: unknown forged tool "stray"`},
		"registered removal":    {tool: RemoveTool(), arguments: `{"name":"csv_stats"}`},
	} {
		t.Run(name, func(t *testing.T) {
			forged := &forgedMemory{tools: []ForgedTool{registered}, verifyErr: test.verifyErr}
			s := turnSession(t)
			if err := s.BeginTurn("work", append(turnTools(), test.tool)); err != nil {
				t.Fatal(err)
			}
			validation := argumentValidationFunc(func(_ root.ToolDefinition, args root.JSONValue) error {
				if test.invalid && strings.Contains(string(args.Bytes()), `"path":5`) {
					return errors.New("path: expected string")
				}
				return nil
			})
			requests := 0
			models := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
				requests++
				if requests == 1 {
					return assistant("", root.ToolCall{ID: "call", Name: test.tool.Definition.Name, Arguments: object(t, test.arguments)}), nil
				}
				return assistant("done"), nil
			})
			u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Models: models, Forge: forged}, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
				t.Fatal("nothing runs before the card")
				return domain.ToolOutcome{}, nil
			})}
			u.Continue.Validation = validation
			if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if test.want == "" {
				if pending := s.Pending(); s.Status() != domain.StatusApproval || len(pending) != 1 {
					t.Fatalf("a valid call waits for its card: %s %+v", s.Status(), pending)
				}
				return
			}
			refused := s.Export().Activity[0]
			if refused.Decision != domain.DecisionDeny || refused.Outcome == nil || !strings.Contains(string(refused.Outcome.Content), test.want) {
				t.Fatalf("not refused before the card with %q: %+v", test.want, refused)
			}
		})
	}
}

// On 20 October 2026 claude-haiku-5.5 sent axlr_forge_tool a why eight
// times, each refused as "arguments must match its schema" without naming
// the field: the forge, remove and install tools accept a why (their
// schemas offer it), and a field they do not know is named in the refusal.
func TestForgeAcceptsAWhyAndNamesAnUnknownField(t *testing.T) {
	forged := &forgedMemory{}
	var args map[string]any
	if err := json.Unmarshal([]byte(forgeArguments), &args); err != nil {
		t.Fatal(err)
	}
	args["why"] = "Crear la herramienta de alarma de costes que pidió la persona."
	encoded, _ := json.Marshal(args)
	host := HostToolUseCase{Forge: forged}
	outcome, err := host.Execute(context.Background(), turnSession(t), hostIdentity(t, domain.HostOperationForgeTool), object(t, string(encoded)))
	if err != nil || outcome.IsError || len(forged.tools) != 1 {
		t.Fatalf("outcome = %s, %v", outcome.Content, err)
	}
	delete(args, "why")
	args["reason"] = "x"
	encoded, _ = json.Marshal(args)
	if _, _, err := parseForge(object(t, string(encoded))); err == nil || !strings.Contains(err.Error(), `unknown field "reason"`) {
		t.Fatalf("refusal does not name the field: %v", err)
	}
	if _, err := decodeInstallRequest(object(t, `{"name":"business_days","why":"instalarla"}`)); err != nil {
		t.Fatal(err)
	}
	if err := removableError(context.Background(), forged, turnSession(t), object(t, `{"name":"word_count","why":"ya no hace falta"}`)); err != nil && strings.Contains(err.Error(), "must be name") {
		t.Fatal(err)
	}
	for _, tool := range RequireWhy([]domain.AvailableTool{ForgeTool(), RemoveTool()}) {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if json.Unmarshal(tool.Definition.Parameters.Bytes(), &schema) != nil || schema.Properties["why"] == nil || slices.Contains(schema.Required, "why") {
			t.Fatalf("%s: why must be offered, not required: %s", tool.Definition.Name, tool.Definition.Parameters.Bytes())
		}
	}
}
