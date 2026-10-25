package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/local"
)

// shellCall asks local_exec, under call id, to run script with sh.
func shellCall(id, script string) string {
	arguments, _ := json.Marshal(map[string]any{"program": "sh", "args": []string{"-c", script}, "why": "The person asked for it."})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "local_exec", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}

// The execution's sandbox confines its commands: under the process
// profile with no egress, a command writes the workspace and nothing
// outside it, the receipt names the sandbox with its digest, and a later
// decision on the session runs under the same sandbox without the flag.
func TestExecRunsItsCommandsUnderTheExecutionsSandbox(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the process profile needs Linux")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err == nil {
		err = local.ProbeSandbox(t.Context(), local.Sandbox{Program: bwrap, Isolate: true})
	}
	if err != nil {
		if os.Getenv("AXLR_SANDBOX_TESTS") == "required" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	workspace, outside := t.TempDir(), t.TempDir()
	escaped := filepath.Join(outside, "escaped")
	calls := scriptedModel(t, shellCall("sh-1", "touch inside && touch "+escaped), answerChunk)
	sandbox := writePolicy(t, `{"sandbox": {"profile": "process"}}`)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the files", "--approve", "local", "--sandbox", sandbox, "--json")
	if code != execCompleted || *calls != 2 || !strings.Contains(stderr, "sandbox process (bwrap)") {
		t.Fatalf("code=%d calls=%d stderr=%s", code, *calls, stderr)
	}
	if _, err := os.Stat(filepath.Join(workspace, "inside")); err != nil {
		t.Fatalf("the workspace write did not land: %v", err)
	}
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Fatalf("a write outside the workspace reached the host: %v", err)
	}
	lines := execLines(t, stdout)
	receipt := lines[len(lines)-2].Receipt
	if receipt == nil || receipt.Sandbox == nil || receipt.Sandbox.Profile != "process" || receipt.Sandbox.Egress != "none" || len(receipt.Sandbox.SHA256) != 64 {
		t.Fatalf("receipt %+v", receipt)
	}
	result := execResult(t, stdout)
	// The session keeps the sandbox: the next turn runs under it.
	calls = scriptedModel(t, shellCall("sh-2", "touch again && touch "+escaped), answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--session", result.Session, "--exec", "again", "--approve", "local", "--json")
	if code != execCompleted || !strings.Contains(stderr, "sandbox process (bwrap)") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Fatalf("the next turn ran unconfined: %v", err)
	}
	if again := execLines(t, stdout); again[len(again)-2].Receipt.Sandbox.SHA256 != receipt.Sandbox.SHA256 {
		t.Fatalf("the sandbox changed: %+v", again[len(again)-2].Receipt.Sandbox)
	}
}

// A sandbox this host cannot give ends the run before its first request,
// never with a weaker one; a file the contract does not describe too.
func TestExecRefusesASandboxItCannotGive(t *testing.T) {
	env := cliEnv(t)
	calls := scriptedModel(t, answerChunk)
	for name, sandbox := range map[string]string{
		"absent image":         `{"profile": "container", "container": {"image": "axlr-sandbox-test/absent:never", "runtime": "podman"}}`,
		"unknown field":        `{"profile": "process", "network": "off"}`,
		"no profile":           `{"egress": "none"}`,
		"allowlist empty":      `{"profile": "process", "egress": "allowlist"}`,
		"allow without list":   `{"profile": "process", "allow": ["example.com"]}`,
		"none with egress":     `{"profile": "none", "egress": "none"}`,
		"container no image":   `{"profile": "container"}`,
		"relative writable":    `{"profile": "process", "writable": ["cache"]}`,
		"bad destination":      `{"profile": "process", "egress": "allowlist", "allow": ["http://example.com"]}`,
		"missing writable dir": `{"profile": "process", "writable": ["/axlr-sandbox-test/absent"]}`,
	} {
		code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done", "--sandbox", writePolicy(t, sandbox))
		if code != execFailed || *calls != 0 || !strings.Contains(stderr, "sandbox") {
			t.Fatalf("%s: code=%d calls=%d stderr=%s", name, code, *calls, stderr)
		}
	}
}
