package axlr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonValue(t *testing.T, s string) root.JSONValue {
	t.Helper()
	v, e := root.NewJSONObject([]byte(s))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func testManager(t *testing.T, marker string) *plugins.Manager {
	t.Helper()
	regs := []plugins.Registration{}
	for _, id := range []string{"alpha", "beta"} {
		r, e := plugins.NewRegistration(plugins.Manifest{ID: root.PluginID(id), Command: os.Args[0], Args: []string{"-test.run=^TestMCPHelper$"}, AllowTools: []root.PluginToolName{"echo"}}, []string{"AXLR_TUI_HELPER=1", "MARKER=" + marker, "IDENTITY=" + id})
		if e != nil {
			t.Fatal(e)
		}
		regs = append(regs, r)
	}
	m, e := plugins.NewManager(regs)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close() })
	return m
}
func TestToolRunnerLocalOperations(t *testing.T) {
	dir := t.TempDir()
	e, err := runtime.New(runtime.Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runner := ToolRunner{Executor: e}
	for _, tc := range []struct{ op, args, want string }{
		{"write", `{"path":"file","content":"hello","mode":"create"}`, "written_bytes"},
		{"read", `{"path":"file"}`, "hello"},
		{"edit", `{"path":"file","old_text":"hello","new_text":"bye"}`, "written_bytes"},
		{"read", `{"path":"file"}`, "bye"},
		{"exec", `{"program":"/bin/echo","args":["worked"]}`, "worked"},
	} {
		id, _ := domain.NewLocalToolIdentity(tc.op)
		out, err := runner.Execute(context.Background(), id, jsonValue(t, tc.args))
		if err != nil || out.IsError || !strings.Contains(string(out.Content), tc.want) {
			t.Fatalf("%s: %+v %v", tc.op, out, err)
		}
	}
	if _, err := runner.Execute(context.Background(), domain.ToolIdentity{}, jsonValue(t, "{}")); err == nil {
		t.Fatal("invalid identity accepted")
	}
	id, _ := domain.NewLocalToolIdentity("read")
	out, err := runner.Execute(context.Background(), id, jsonValue(t, `{"path":"absent"}`))
	if err != nil || !out.IsError || out.Uncertain {
		t.Fatalf("%+v %v", out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err = runner.Execute(ctx, id, jsonValue(t, `{"path":"file"}`))
	if err != nil || !out.IsError || !out.Uncertain {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestToolRunnerMCPErrorIdentityAndDisappearance(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "gone")
	m := testManager(t, marker)
	e, err := runtime.New(runtime.Config{Root: t.TempDir(), Plugins: m})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runner := ToolRunner{Executor: e}
	snapshot, err := (ToolCatalog{Plugins: m}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range snapshot[4:] {
		id, err := ResolveTool(snapshot, tool.Definition.Name)
		if err != nil {
			t.Fatal(err)
		}
		out, err := runner.Execute(context.Background(), id, jsonValue(t, `{"n":9007199254740993}`))
		if err != nil || !out.IsError || out.Uncertain || !strings.Contains(string(out.Content), id.Plugin.PluginID.String()+":echo:9007199254740993") {
			t.Fatalf("%+v %v", out, err)
		}
	}
	if err := os.WriteFile(marker, []byte("gone"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runner.Execute(context.Background(), snapshot[4].Identity, jsonValue(t, "{}"))
	if err != nil || !out.IsError || out.Uncertain || !strings.Contains(string(out.Content), "unknown_plugin_tool") {
		t.Fatalf("%+v %v", out, err)
	}
	fresh, err := (ToolCatalog{Plugins: m}).Snapshot(context.Background())
	if err != nil || len(fresh) != 4 {
		t.Fatalf("fresh: %+v %v", fresh, err)
	}
	if _, err := ResolveTool(snapshot, snapshot[4].Definition.Name); err != nil {
		t.Fatal("old snapshot was changed", err)
	}
	if _, err := ResolveTool(fresh, snapshot[4].Definition.Name); err == nil {
		t.Fatal("missing tool present in fresh snapshot")
	}
	id, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "alpha", ToolName: "hidden"})
	out, err = runner.Execute(context.Background(), id, jsonValue(t, "{}"))
	if err != nil || !out.IsError || out.Uncertain || !strings.Contains(string(out.Content), "not allowed") {
		t.Fatalf("%+v %v", out, err)
	}
	calls, err := os.ReadFile(marker + ".calls")
	if err != nil || string(calls) != "alpha:echo\nbeta:echo\n" {
		t.Fatalf("executor calls %q %v", calls, err)
	}

}
func TestMCPHelper(t *testing.T) {
	if os.Getenv("AXLR_TUI_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Name      string
				Arguments map[string]json.RawMessage
			}
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		if len(req.ID) == 0 {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "test", "version": "1"}}
		case "tools/list":
			list := []any{}
			if _, err := os.Stat(os.Getenv("MARKER")); os.IsNotExist(err) {
				for _, name := range []string{"echo", "hidden"} {
					list = append(list, map[string]any{"name": name, "inputSchema": map[string]any{"type": "object"}})
				}
			}
			result = map[string]any{"tools": list}
		case "tools/call":
			if marker := os.Getenv("MARKER"); marker != "" {
				f, err := os.OpenFile(marker+".calls", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					os.Exit(3)
				}
				fmt.Fprintln(f, os.Getenv("IDENTITY")+":"+req.Params.Name)
				f.Close()
			}
			if string(req.Params.Arguments["lose_reply"]) == "true" {
				os.Exit(0)
			}
			result = map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": os.Getenv("IDENTITY") + ":" + req.Params.Name + ":" + string(req.Params.Arguments["n"])}}}
		default:
			result = map[string]any{}
		}
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		fmt.Println(string(data))
	}
	os.Exit(0)
}

// A plugin effect followed by EOF must not become a definite result or resume the model.
func TestToolRunnerLostMCPReplyPausesPersistedTurn(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "effect")
	manager := testManager(t, marker)
	executor, err := runtime.New(runtime.Config{Root: t.TempDir(), Plugins: manager})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	snapshot, err := (ToolCatalog{Plugins: manager}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
	if err != nil {
		t.Fatal(err)
	}
	if err = session.BeginTurn("run plugin", snapshot); err != nil {
		t.Fatal(err)
	}
	call := root.ToolCall{ID: "lost", Name: snapshot[4].Definition.Name, Arguments: jsonValue(t, `{"lose_reply":true}`)}
	if err = session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	model := &countingStream{}
	resolver := application.ResolveToolUseCase{Tools: ToolRunner{Executor: executor}, Store: store, Continue: application.ContinueTurnUseCase{Models: model}}
	runErr := resolver.Execute(context.Background(), &session, call.ID, domain.DecisionApprove, nil)
	effect, err := os.ReadFile(marker + ".calls")
	if err != nil || string(effect) != "alpha:echo\n" {
		t.Fatalf("effect %q: %v", effect, err)
	}
	saved, err := store.Load(context.Background(), session.Export().ID)
	if err != nil {
		t.Fatal(err)
	}
	outcome := saved.Export().Activity[0].Outcome
	if runErr == nil || model.calls != 0 || session.Status() != domain.StatusInterrupted || saved.Status() != domain.StatusInterrupted || outcome == nil || !outcome.Uncertain || !outcome.IsError {
		t.Fatalf("lost reply: err=%v model calls=%d live=%s saved=%s outcome=%+v", runErr, model.calls, session.Status(), saved.Status(), outcome)
	}
}

type countingStream struct{ calls int }

func (m *countingStream) Stream(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
	m.calls++
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "incorrect automatic continuation"}}, nil
}
