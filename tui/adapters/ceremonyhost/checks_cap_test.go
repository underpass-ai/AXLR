package ceremonyhost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	axlrruntime "github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A pull request with 60 check runs shaped like AXLR's own (about 300 bytes
// each, measured on PR 101) prints a view larger than the 16 KiB cap the
// model's checks get. Through the real runtime the console must still read it
// whole.
func TestForgeStatusReadsSixtyChecksThroughTheRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	work, bin := t.TempDir(), t.TempDir()
	var checks []string
	for i := 1; i <= 60; i++ {
		checks = append(checks, fmt.Sprintf(`{"__typename":"CheckRun","completedAt":"2026-10-08T23:01:55Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/underpass-ai/AXLR/actions/runs/37857073289/job/1135837033%02d","name":"ceremonies-%02d","startedAt":"2026-10-08T23:01:45Z","status":"COMPLETED","workflowName":"CI"}`, i, i))
	}
	view := `{"headRefOid":"abc","mergeStateStatus":"CLEAN","state":"OPEN","statusCheckRollup":[` + strings.Join(checks, ",") + `]}` + "\n"
	if len(view) <= checkOutput {
		t.Fatalf("fixture is %d bytes, must exceed the %d-byte model check cap", len(view), checkOutput)
	}
	if err := os.WriteFile(filepath.Join(bin, "view.json"), []byte(view), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\ncat "+filepath.Join(bin, "view.json")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	executor, err := axlrruntime.New(axlrruntime.Config{Root: work, Env: []string{"PATH=" + bin + ":/usr/bin:/bin"}})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	forge := Forge{Checks: Checks{Tools: axlr.ToolRunner{Executor: executor}}}
	status, err := forge.Status(context.Background(), "underpass-ai/AXLR", 101)
	if err != nil {
		t.Fatalf("a %d-byte pull request view must be readable: %v", len(view), err)
	}
	if status.State != "OPEN" || status.HeadSHA != "abc" || status.Passed != 60 || status.Pending != 0 || len(status.Failed) != 0 {
		t.Fatalf("status %+v", status)
	}
}

// truncatedPort answers like the runtime when the output cap cut the
// command's output: exit 0, a cut stdout and truncated=true.
type truncatedPort struct {
	stdout string
	asked  *int
}

func (p truncatedPort) Execute(_ context.Context, _ domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	var request struct {
		MaxOutputBytes int `json:"max_output_bytes"`
	}
	_ = json.Unmarshal(args.Bytes(), &request)
	if p.asked != nil {
		*p.asked = request.MaxOutputBytes
	}
	encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"exit_code": 0, "stdout": p.stdout, "stderr": "", "truncated": true}})
	return domain.ToolOutcome{Content: root.Text(encoded)}, nil
}

// A view cut by the cap is refused by name, not parsed into a JSON error, and
// the console's own commands ask the runtime for its whole output limit while
// the model's checks keep their smaller cap.
func TestForgeRefusesOutputTheRuntimeCutAndAsksForTheWholeLimit(t *testing.T) {
	asked := 0
	runner := Checks{Tools: truncatedPort{stdout: `{"state":"OPEN","statusCheckRollup":[{"name":"a"`, asked: &asked}}
	_, err := Forge{Checks: runner}.Status(context.Background(), "o/r", 7)
	if err == nil || !strings.Contains(err.Error(), "output was cut") {
		t.Fatalf("a cut view must be refused by name: %v", err)
	}
	if asked != consoleOutput {
		t.Fatalf("forge asked for %d output bytes, want %d", asked, consoleOutput)
	}
	result, err := runner.Run(context.Background(), domain.CheckCommand{Program: "go", Args: []string{"test"}})
	if err != nil || !result.Truncated || asked != checkOutput {
		t.Fatalf("model check: truncated=%v asked=%d err=%v", result.Truncated, asked, err)
	}
	if _, err := runner.Run(context.Background(), domain.CheckCommand{Program: "git", MaxOutput: 8 << 20}); err != nil || asked != consoleOutput {
		t.Fatalf("a cap above the runtime's limit must be clamped to it: asked=%d err=%v", asked, err)
	}
}
