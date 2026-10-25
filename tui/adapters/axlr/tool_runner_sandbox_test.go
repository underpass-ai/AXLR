package axlr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A command of a session with a sandbox runs under the runtime's sandbox
// Confine gives for it, or not at all: never unconfined. Other tools are
// not the sandbox's.
func TestTheToolRunnerConfinesAnExecutionsCommands(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.New(runtime.Config{Root: workspace, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(workspace), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetSandbox(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressNone}); err != nil {
		t.Fatal(err)
	}
	ctx := application.WithSessionSandbox(context.Background(), session)
	exec, _ := domain.NewLocalToolIdentity("exec")
	read, _ := domain.NewLocalToolIdentity("read")
	command := jsonValue(t, `{"program":"touch","args":["ran"]}`)

	outcome, err := ToolRunner{Executor: executor}.Execute(ctx, exec, command)
	if err != nil || !outcome.IsError || outcome.Uncertain || !strings.Contains(string(outcome.Content), `"code":"sandbox_unavailable"`) {
		t.Fatalf("without Confine: %+v %v", outcome, err)
	}
	var seen []domain.ExecutionSandbox
	refusing := ToolRunner{Executor: executor, Confine: func(sandbox domain.ExecutionSandbox) (*local.Sandbox, error) {
		seen = append(seen, sandbox)
		return nil, errors.New("bwrap is not installed")
	}}
	outcome, err = refusing.Execute(ctx, exec, command)
	if err != nil || !outcome.IsError || !strings.Contains(string(outcome.Content), "the execution's sandbox is unavailable, so the command did not run: bwrap is not installed") {
		t.Fatalf("Confine refusing: %+v %v", outcome, err)
	}
	confined := ToolRunner{Executor: executor, Confine: func(sandbox domain.ExecutionSandbox) (*local.Sandbox, error) {
		seen = append(seen, sandbox)
		return &local.Sandbox{Unavailable: "the test's sandbox"}, nil
	}}
	outcome, err = confined.Execute(ctx, exec, command)
	if err != nil || !outcome.IsError || !strings.Contains(string(outcome.Content), "the test's sandbox") {
		t.Fatalf("under the runtime's sandbox: %+v %v", outcome, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "ran")); !os.IsNotExist(err) {
		t.Fatalf("a refused command ran: %v", err)
	}
	if len(seen) != 2 || seen[0].Profile != domain.SandboxProfileProcess {
		t.Fatalf("Confine saw %+v", seen)
	}
	outcome, err = ToolRunner{Executor: executor}.Execute(ctx, read, jsonValue(t, `{"path":"notes.txt"}`))
	if err != nil || outcome.IsError || !strings.Contains(string(outcome.Content), "kept") {
		t.Fatalf("a read under the sandbox: %+v %v", outcome, err)
	}
}
