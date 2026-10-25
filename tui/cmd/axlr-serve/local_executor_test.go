package main

import (
	"context"
	"encoding/json"
	goruntime "runtime"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 25 October 2026 every local_exec on the service failed "not found in
// configured PATH": its executor had no environment. A service local_exec
// now resolves a system program with the PATH the service inherited, or
// with the system folders when a unit gives it none, and sees no
// credential.
func TestServiceLocalExecResolvesASystemProgram(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario; the Windows PATH is covered by compose tests")
	}
	manager, err := plugins.NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	id, err := domain.NewLocalToolIdentity("exec")
	if err != nil {
		t.Fatal(err)
	}
	args, err := root.NewJSONValue([]byte(`{"program":"sh","args":["-c","printf path-ok; test -z \"$OPENROUTER_API_KEY$XAI_API_KEY\""]}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"inherited": "/usr/bin:/bin", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			env := map[string]string{"PATH": path, "HOME": t.TempDir(), "OPENROUTER_API_KEY": "private-key", "XAI_API_KEY": "xai-never-print"}
			executor, err := localExecutor(t.TempDir(), manager, nil, func(name string) string { return env[name] })
			if err != nil {
				t.Fatal(err)
			}
			defer executor.Close()
			outcome, err := axlr.ToolRunner{Executor: executor}.Execute(context.Background(), id, args)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Status string         `json:"status"`
				Output dto.ExecOutput `json:"output"`
			}
			if err := json.Unmarshal([]byte(outcome.Content), &response); err != nil {
				t.Fatal(err)
			}
			if outcome.IsError || response.Status != "completed" || response.Output.ExitCode != 0 || response.Output.Stdout != "path-ok" {
				t.Fatalf("sh unavailable or a credential forwarded: %s", outcome.Content)
			}
		})
	}
}
