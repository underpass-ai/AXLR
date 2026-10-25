package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deliverCall asks axlr_deliver, under call id, to hand over arguments.
func deliverCall(id string, arguments map[string]any) string {
	encoded, _ := json.Marshal(arguments)
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "axlr_deliver", "arguments": string(encoded)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}

// A run delivers a workspace file by path without a card, and inline content
// under --approve local: each is an artifact line after its call and an
// entry of the result's artifacts, with its size, sha256 and the reference
// the control plane fetches. Under --approve none the inline delivery waits
// for a decision, as a write does.
func TestExecDeliversArtifacts(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "report.md"), []byte("# Report\n"), 0644); err != nil {
		t.Fatal(err)
	}
	byPath := deliverCall("deliver-1", map[string]any{"why": "The task asked for the report.", "name": "report.md", "media_type": "text/markdown", "description": "The report", "path": "report.md"})
	inline := deliverCall("deliver-2", map[string]any{"why": "The task asked for the data.", "name": "data.json", "media_type": "application/json", "description": "The numbers", "content": `{"errors":3}`})
	scriptedModel(t, byPath, inline, answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "deliver", "--approve", "local", "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	result := execResult(t, stdout)
	reference := ".axlr/deliverables/" + result.Session + "/data.json"
	if len(result.Artifacts) != 2 || result.Artifacts[0].Reference != "report.md" || result.Artifacts[0].Source != "file" || result.Artifacts[0].Size != 9 || result.Artifacts[1].Reference != reference || result.Artifacts[1].Source != "inline" || result.Artifacts[1].Call != "deliver-2" || len(result.Artifacts[1].SHA256) != 64 {
		t.Fatalf("%+v", result.Artifacts)
	}
	if data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(reference))); err != nil || string(data) != `{"errors":3}` {
		t.Fatalf("%q %v", data, err)
	}
	var artifacts []string
	for i, line := range execLines(t, stdout) {
		if line.Type == "artifact" {
			if line.Artifact == nil || execLines(t, stdout)[i-1].Type != "tool.completed" {
				t.Fatalf("artifact line %+v", line)
			}
			artifacts = append(artifacts, line.Artifact.Reference)
		}
	}
	if strings.Join(artifacts, ",") != "report.md,"+reference {
		t.Fatalf("artifact lines %v", artifacts)
	}
	scriptedModel(t, inline, answerChunk)
	code, stdout, _ = execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "deliver", "--approve", "none", "--json")
	if waiting := execResult(t, stdout); code != execNeedsDecision || waiting.Pending == nil || waiting.Pending.Tool != "axlr_deliver" || len(waiting.Artifacts) != 0 {
		t.Fatalf("code=%d %+v", code, waiting)
	}
}

// Under an observing policy the inline delivery is refused outside the
// model, while a delivery by path goes on.
func TestExecPolicyObservesDeliveries(t *testing.T) {
	env := cliEnv(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "report.md"), []byte("# Report\n"), 0644); err != nil {
		t.Fatal(err)
	}
	inline := deliverCall("deliver-1", map[string]any{"why": "w", "name": "plan.md", "media_type": "text/markdown", "description": "d", "content": "x"})
	byPath := deliverCall("deliver-2", map[string]any{"why": "w", "name": "report.md", "media_type": "text/markdown", "description": "d", "path": "report.md"})
	scriptedModel(t, inline, byPath, answerChunk)
	policy := writePolicy(t, `{"level": "observe"}`)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "deliver", "--approve", "all", "--policy", policy, "--json")
	result := execResult(t, stdout)
	if code != execCompleted || len(result.Calls) != 2 || result.Calls[0].Status != "forbidden" || result.Calls[1].Status != "completed" || len(result.Artifacts) != 1 || result.Artifacts[0].Reference != "report.md" {
		t.Fatalf("code=%d stderr=%s %+v", code, stderr, result)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".axlr")); !os.IsNotExist(err) {
		t.Fatalf("the forbidden delivery wrote: %v", err)
	}
}
