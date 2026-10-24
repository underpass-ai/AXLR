package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/dto"
)

// execCall asks local_exec, under call id, to create made-by-exec in the
// workspace, as execChunk does.
func execCall(id string) string {
	program, args := "touch", []string{"made-by-exec"}
	if runtime.GOOS == "windows" {
		program, args = "cmd", []string{"/c", "mkdir", "made-by-exec"}
	}
	arguments, _ := json.Marshal(map[string]any{"program": program, "args": args, "why": "The person asked for the file."})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "local_exec", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}

// execLines reads every --json line of a run.
func execLines(t *testing.T, stdout string) []execEvent {
	t.Helper()
	var events []execEvent
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var event execEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

// The budget ends the run outside the model at the first limit reached:
// the request past the requests limit is never sent, and the call past
// the tool_calls limit never runs and stays pending, so a later run under
// a new budget decides it. Both end budget_exceeded with exit 1.
func TestExecEndsAtTheBudgetAndResumesUnderANewOne(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	workspace := t.TempDir()
	calls := scriptedModel(t, execChunk, answerChunk)
	budget := writePolicy(t, `{"budget": {"requests": 1}}`)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file", "--approve", "local", "--budget", budget, "--json")
	if code != execFailed || *calls != 1 || !strings.Contains(stderr, "budget: requests 1 reached") {
		t.Fatalf("code=%d calls=%d stderr=%s", code, *calls, stderr)
	}
	lines := execLines(t, stdout)
	result := execResult(t, stdout)
	if result.Status != "budget_exceeded" || result.Error != "budget: requests 1 reached" || result.Pending != nil || len(result.Calls) != 1 || result.Calls[0].Status != "completed" || result.Usage.Requests != 1 {
		t.Fatalf("%+v", result)
	}
	if failed := lines[len(lines)-2]; failed.Type != "failed" || failed.Error != "budget: requests 1 reached" {
		t.Fatalf("%+v", failed)
	}
	if _, err := os.Stat(filepath.Join(workspace, "made-by-exec")); err != nil {
		t.Fatalf("the call within the budget did not run: %v", err)
	}
	// The tool_calls limit: the second call of the run stays pending.
	workspace = t.TempDir()
	calls = scriptedModel(t, execCall("exec-1"), execCall("exec-2"), answerChunk)
	budget = writePolicy(t, `{"tool_calls": 1}`)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file twice", "--approve", "local", "--budget", budget, "--json")
	result = execResult(t, stdout)
	if code != execFailed || *calls != 2 || result.Status != "budget_exceeded" || result.Error != "budget: tool_calls 1 reached" || result.Pending == nil || result.Pending.ID != "exec-2" || len(result.Calls) != 2 || result.Calls[0].Status != "completed" || result.Calls[1].Status != "pending" {
		t.Fatalf("code=%d calls=%d stderr=%s %+v", code, *calls, stderr, result)
	}
	// Decided under the spent budget, the call still does not run.
	calls = scriptedModel(t, answerChunk)
	code, stdout, _ = execRun(t, env, "--root", workspace, "--session", result.Session, "--decide", "approve", "--call", "exec-2", "--json")
	if again := execResult(t, stdout); code != execFailed || *calls != 0 || again.Status != "budget_exceeded" || again.Pending == nil || again.Pending.ID != "exec-2" {
		t.Fatalf("code=%d calls=%d %+v", code, *calls, again)
	}
	// Under a new budget it runs and the turn ends.
	calls = scriptedModel(t, answerChunk)
	wider := writePolicy(t, `{"tool_calls": 10, "requests": 10}`)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--session", result.Session, "--decide", "approve", "--call", "exec-2", "--budget", wider, "--json")
	if done := execResult(t, stdout); code != execCompleted || *calls != 1 || done.Status != "completed" || len(done.Calls) != 2 || done.Calls[1].Status != "completed" || done.Usage.Requests != 3 {
		t.Fatalf("code=%d calls=%d stderr=%s %+v", code, *calls, stderr, done)
	}
}

// The correlation goes on every --json line, in the result and on every
// record of the run's trace, unchanged.
func TestExecCarriesTheCorrelation(t *testing.T) {
	env := cliEnv(t)
	scriptedModel(t, answerChunk)
	correlation := writePolicy(t, `{"correlation": {"bot_id": "server-incident-investigator", "task_id": "task-789", "execution_id": "exec-1", "trace_id": "trace-abc"}}`)
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done", "--correlation", correlation, "--trace-file", trace, "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	want := dto.ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}
	lines := execLines(t, stdout)
	for _, line := range lines {
		if line.Correlation == nil || *line.Correlation != want {
			t.Fatalf("line %s without the correlation: %+v", line.Type, line.Correlation)
		}
	}
	if result := execResult(t, stdout); result.Correlation == nil || *result.Correlation != want {
		t.Fatalf("%+v", result)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	records := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(records) < 3 {
		t.Fatalf("trace %d records", len(records))
	}
	for _, record := range records {
		var event struct {
			Correlation *dto.ExecutionCorrelation `json:"correlation"`
		}
		if err := json.Unmarshal([]byte(record), &event); err != nil || event.Correlation == nil || *event.Correlation != want {
			t.Fatalf("trace record without the correlation: %s (%v)", record, err)
		}
	}
	// A run without one writes no correlation anywhere.
	scriptedModel(t, answerChunk)
	_, stdout, _ = execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done", "--json")
	if strings.Contains(stdout, "correlation") {
		t.Fatalf("a correlation from nowhere: %s", stdout)
	}
}

// A budget or correlation file the contract does not describe stops the
// run before any request.
func TestExecRefusesInvalidBudgetAndCorrelationFiles(t *testing.T) {
	env := cliEnv(t)
	calls := scriptedModel(t, answerChunk)
	for name, args := range map[string][]string{
		"budget unknown field":    {"--budget", writePolicy(t, `{"seconds": 1, "minutes": 1}`)},
		"budget negative":         {"--budget", writePolicy(t, `{"requests": -1}`)},
		"budget not json":         {"--budget", writePolicy(t, `900`)},
		"budget missing":          {"--budget", filepath.Join(t.TempDir(), "missing.json")},
		"correlation unsafe id":   {"--correlation", writePolicy(t, `{"bot_id": "bot one"}`)},
		"correlation unknown key": {"--correlation", writePolicy(t, `{"span_id": "s"}`)},
		"correlation missing":     {"--correlation", filepath.Join(t.TempDir(), "missing.json")},
	} {
		code, _, stderr := execRun(t, env, append([]string{"--root", t.TempDir(), "--model", "test/model", "--exec", "say done"}, args...)...)
		if code != execFailed || !strings.Contains(stderr, args[0]) || *calls != 0 {
			t.Fatalf("%s: code=%d calls=%d stderr=%s", name, code, *calls, stderr)
		}
	}
}
