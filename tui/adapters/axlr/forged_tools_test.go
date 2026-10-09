package axlr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The model forges a tool and runs it in the same session: the store, the
// argument validator and the local runtime are the console's own.
func TestAForgedToolRunsAtOnceThroughTheRuntime(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the forged tool is a shell script")
	}
	workspace := t.TempDir()
	executor, err := runtime.New(runtime.Config{Root: workspace, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	store, err := storage.NewForgedToolStore(filepath.Join(t.TempDir(), "forged"))
	if err != nil {
		t.Fatal(err)
	}
	host := application.HostToolUseCase{Forge: store, Tools: ToolRunner{Executor: executor}, Validation: NewToolArgumentValidator()}
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(workspace), "model")
	if err != nil {
		t.Fatal(err)
	}
	forge, _ := domain.NewHostToolIdentity(domain.HostOperationForgeTool)
	run, _ := domain.NewHostToolIdentity(domain.HostOperationRunTool)
	tools, _ := domain.NewHostToolIdentity(domain.HostOperationTools)

	script := `read input; printf '%s' "$input" | tr -cd 'a-z' | wc -c | tr -d ' '`
	forged, err := host.Execute(context.Background(), session, forge, jsonValue(t, `{"name":"count_letters","description":"Count the lowercase letters of word.","input_schema":{"type":"object","properties":{"word":{"type":"string"}},"required":["word"],"additionalProperties":false},"program":"sh","args":[".axlr/tools/count_letters/count.sh"],"files":[{"path":"count.sh","content":`+quote(script)+`}]}`))
	if err != nil || forged.IsError {
		t.Fatalf("forge = %+v, %v", forged, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".axlr", "tools", "count_letters", "count.sh")); err != nil {
		t.Fatalf("tool file not written: %v", err)
	}

	listed, err := host.Execute(context.Background(), session, tools, jsonValue(t, `{"query":"letters"}`))
	if err != nil || !strings.Contains(string(listed.Content), `"forged":true`) || !strings.Contains(string(listed.Content), "count_letters") {
		t.Fatalf("axlr_tools does not list the forged tool: %s, %v", listed.Content, err)
	}

	// The script reads the whole JSON object on stdin: word, abc and gh.
	result, err := host.Execute(context.Background(), session, run, jsonValue(t, `{"name":"count_letters","arguments":{"word":"abc DEF gh"}}`))
	if err != nil || result.IsError || !strings.Contains(string(result.Content), `"stdout":"9\n"`) {
		t.Fatalf("run = %+v, %v", result, err)
	}

	refused, _ := host.Execute(context.Background(), session, run, jsonValue(t, `{"name":"count_letters","arguments":{"text":"abc"}}`))
	if !refused.IsError || !strings.Contains(string(refused.Content), "input_schema") {
		t.Fatalf("arguments outside the schema ran: %s", refused.Content)
	}

	// Code changed outside axlr_forge_tool is not the code that was approved.
	if err := os.WriteFile(filepath.Join(workspace, ".axlr", "tools", "count_letters", "count.sh"), []byte("echo pwned"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _ := host.Execute(context.Background(), session, run, jsonValue(t, `{"name":"count_letters","arguments":{"word":"abc"}}`))
	if !changed.IsError || !strings.Contains(string(changed.Content), "changed since it was forged") {
		t.Fatalf("a changed tool ran: %s", changed.Content)
	}
}

func quote(text string) string {
	encoded, _ := json.Marshal(text)
	return string(encoded)
}
