package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

// failingExecChunk asks local_exec to run a program that exits 1.
var failingExecChunk = func() string {
	program, args := "false", []string{}
	if runtime.GOOS == "windows" {
		program, args = "cmd", []string{"/c", "exit", "1"}
	}
	arguments, _ := json.Marshal(map[string]any{"program": program, "args": args, "why": "The person asked to see it fail."})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "exec-1", "type": "function", "function": map[string]any{"name": "local_exec", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}()

// execLogs is the run's app log and every trace it wrote.
func execLogs(t *testing.T, env map[string]string) (string, string) {
	t.Helper()
	dir := filepath.Join(env["XDG_STATE_HOME"], "axlr", "logs")
	appLog, err := os.ReadFile(filepath.Join(dir, "axlr.log"))
	if err != nil {
		t.Fatal(err)
	}
	traces, _ := filepath.Glob(filepath.Join(dir, "trace-*.jsonl"))
	var trace strings.Builder
	for _, path := range traces {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		trace.Write(data)
	}
	return string(appLog), trace.String()
}

// completedEvent is the run's tool.completed line for call.
func completedEvent(t *testing.T, stdout, call string) execEvent {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var event execEvent
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "tool.completed" && event.Call == call {
			return event
		}
	}
	t.Fatalf("no tool.completed for %s in %s", call, stdout)
	return execEvent{}
}

// An --exec run writes one app log line per finished call: a call that
// ran and failed at WARN with what it answered, its trace record classed
// as the call's own answer and its event with that answer whole; a call
// the policy refused before it ran at WARN too. Until 12 November 2026 the
// log had no line for a tool call and an operator could not see a bot's
// refused calls.
func TestExecLogsEveryFinishedToolCall(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	workspace := t.TempDir()
	scriptedModel(t, failingExecChunk, answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "run it", "--approve", "all", "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	result := execResult(t, stdout)
	event := completedEvent(t, stdout, "exec-1")
	if event.Status != "failed" || event.Error == "" || strings.Contains(event.Error, "\n") {
		t.Fatalf("event %+v", event)
	}
	appLog, trace := execLogs(t, env)
	line := "WARN  "
	want := "tool call: session " + result.Session + ", call exec-1, tool local_exec, decision auto_approve, failed in "
	found := false
	for _, entry := range strings.Split(appLog, "\n") {
		if strings.Contains(entry, want) {
			found = strings.Contains(entry, line) && strings.Contains(entry, application.ToolCallErrorText(event.Error, 1536))
			break
		}
	}
	if !found {
		t.Fatalf("no WARN line %q with the error in\n%s", want, appLog)
	}
	if strings.Contains(appLog, "The person asked to see it fail.") {
		t.Fatal("the app log carries the call's arguments")
	}
	if classes := completionClasses(t, trace); len(classes) != 1 || classes[0] != "tool_result" {
		t.Fatalf("tool_completed classes %q in\n%s", classes, trace)
	}

	// The person's decision goes through the console's resolver: a run
	// that stops at the card, then --decide approve on its session.
	env = cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	scriptedModel(t, failingExecChunk, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "run it", "--json")
	if code != execNeedsDecision {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	pending := execResult(t, stdout)
	scriptedModel(t, answerChunk)
	code, _, stderr = execRun(t, env, "--root", workspace, "--session", pending.Session, "--decide", "approve", "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	appLog, _ = execLogs(t, env)
	if want := "tool call: session " + pending.Session + ", call exec-1, tool local_exec, decision approve, failed in "; !strings.Contains(appLog, want) {
		t.Fatalf("no line %q in\n%s", want, appLog)
	}

	// A call the policy refuses before it runs is logged as well.
	env = cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, _ = io.ReadAll(r.Body)
		requests++
		chunk := answerChunk
		if requests == 1 {
			chunk = execChunk
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	policy := writePolicy(t, `{"policy": {"level": "observe", "tools": {"allow": [{"name": "local_read"}]}}}`)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file", "--approve", "all", "--policy", policy, "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	refused := completedEvent(t, stdout, "exec-1")
	if refused.Status != "failed" || !strings.Contains(refused.Error, "local_exec is not in the execution's allow list") {
		t.Fatalf("refused %+v", refused)
	}
	appLog, _ = execLogs(t, env)
	if want := "tool call: session " + execResult(t, stdout).Session + ", call exec-1, tool local_exec, decision deny, failed in 0s: " + refused.Error; !strings.Contains(appLog, want) {
		t.Fatalf("no line %q in\n%s", want, appLog)
	}
}

// completionClasses is the error class of each tool_completed record.
func completionClasses(t *testing.T, trace string) []string {
	t.Helper()
	var classes []string
	for _, line := range strings.Split(strings.TrimSpace(trace), "\n") {
		var record struct {
			Stage      string `json:"stage"`
			ErrorClass string `json:"error_class"`
		}
		if json.Unmarshal([]byte(line), &record) == nil && record.Stage == "tool_completed" {
			classes = append(classes, record.ErrorClass)
		}
	}
	return classes
}
