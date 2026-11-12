package main

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
)

// failingExecCall asks local_exec, as call id, to run a program that
// exits 1.
func failingExecCall(id string) string {
	program, args := "false", []string{}
	if runtime.GOOS == "windows" {
		program, args = "cmd", []string{"/c", "exit", "1"}
	}
	arguments, _ := json.Marshal(map[string]any{"program": program, "args": args, "why": "The model tries it again."})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "local_exec", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}

// An --exec run whose model sends the same failing call again and again
// ends at the settings' tool_failure_streak, not at the turn's budget: the
// result and the receipt say failed with the tool loop named, and the app
// log has a WARN line for it. On 12 November 2026 a bot repeated a refused
// axlr_remember ten times until someone cancelled it by hand.
func TestExecStopsAModelThatRepeatsTheSameFailure(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	writeSettings(t, env, `{"tool_failure_streak":2}`)
	requests := scriptedModel(t, failingExecCall("exec-1"), failingExecCall("exec-2"), failingExecCall("exec-3"), answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "run it", "--approve", "all", "--json")
	if code != execFailed || *requests != 2 {
		t.Fatalf("code=%d requests=%d stderr=%s", code, *requests, stderr)
	}
	result := execResult(t, stdout)
	want := "tool loop: local_exec failed 2 times in a row with the same error"
	if result.Status != "failed" || !strings.HasPrefix(result.Error, want) {
		t.Fatalf("result %+v", result)
	}
	var receipt execEvent
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var event execEvent
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "receipt" {
			receipt = event
		}
	}
	if receipt.Receipt == nil || receipt.Receipt.Status != "failed" || receipt.Receipt.Error != result.Error {
		t.Fatalf("receipt %+v", receipt.Receipt)
	}
	appLog, _ := execLogs(t, env)
	found := false
	for _, entry := range strings.Split(appLog, "\n") {
		if strings.Contains(entry, want) {
			found = strings.Contains(entry, "WARN  ")
			break
		}
	}
	if !found {
		t.Fatalf("no WARN line %q in\n%s", want, appLog)
	}
}
