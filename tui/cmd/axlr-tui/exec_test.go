package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/underpass-ai/AXLR/tui/dto"
)

// scriptedModel answers each request with the next chunk, over the default
// transport the console's model client uses.
func scriptedModel(t *testing.T, chunks ...string) *int {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, _ = io.ReadAll(r.Body)
		chunk := chunks[min(calls, len(chunks)-1)]
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	return &calls
}

func execRun(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	previous := execOutput
	execOutput = &stdout
	defer func() { execOutput = previous }()
	code := run(context.Background(), args, func(k string) string { return env[k] }, func(tea.Model) error {
		t.Fatal("--exec launched the console")
		return nil
	}, &stderr)
	return code, stdout.String(), stderr.String()
}

const (
	answerChunk = `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`
)

// execChunk asks local_exec to create made-by-exec in the workspace with a
// program every runner has: touch, or cmd's mkdir on Windows.
var execChunk = func() string {
	program, args := "touch", []string{"made-by-exec"}
	if runtime.GOOS == "windows" {
		program, args = "cmd", []string{"/c", "mkdir", "made-by-exec"}
	}
	arguments, _ := json.Marshal(map[string]any{"program": program, "args": args, "why": "The person asked for the file."})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "exec-1", "type": "function", "function": map[string]any{"name": "local_exec", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}()

// --exec runs one prompt without the console and prints the answer on
// stdout; the session id goes to stderr for --session.
func TestExecPrintsTheAnswer(t *testing.T) {
	env := cliEnv(t)
	scriptedModel(t, answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done")
	if code != execCompleted || stdout != "done\n" || !strings.Contains(stderr, "axlr-tui: session ") {
		t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
}

// A command needs a decision: --exec stops there with exit 3 and runs
// nothing, unless --approve local lets the run take it.
func TestExecStopsAtADecisionUnlessAllowed(t *testing.T) {
	env := cliEnv(t)
	// The command runs with the console's PATH; without one the console
	// falls back to Unix directories, where Windows has no cmd.
	env["PATH"] = os.Getenv("PATH")
	workspace := t.TempDir()
	scriptedModel(t, execChunk, answerChunk)
	code, _, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file")
	if code != execNeedsDecision || !strings.Contains(stderr, "waits for a decision on local_exec") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(workspace, "made-by-exec")); err == nil {
		t.Fatal("the command ran without a decision")
	}
	calls := scriptedModel(t, execChunk, answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file", "--approve", "local", "--json")
	if code != execCompleted || *calls != 2 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, *calls, stderr)
	}
	if _, err := os.Stat(filepath.Join(workspace, "made-by-exec")); err != nil {
		t.Fatalf("the approved command did not run: %v", err)
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var event execEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		types = append(types, event.Type)
	}
	if joined := strings.Join(types, " "); !strings.HasPrefix(joined, "session") || !strings.Contains(joined, "tool.completed") || !strings.HasSuffix(joined, "completed result") {
		t.Fatalf("events %s", joined)
	}
}

// execResult reads the result line a --json run ends with.
func execResult(t *testing.T, stdout string) dto.ExecutionResult {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var last execEvent
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil || last.Type != "result" || last.Result == nil {
		t.Fatalf("last line %q: %v", lines[len(lines)-1], err)
	}
	return *last.Result
}

// The run ends with the execution's result: the answer, the calls, the
// usage of the run's requests. A program reads that line alone.
func TestExecWritesTheResultLast(t *testing.T) {
	env := cliEnv(t)
	scriptedModel(t, `{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"cost":0.0004}}`)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done", "--json")
	if code != execCompleted {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	result := execResult(t, stdout)
	if result.Schema != dto.ExecutionResultSchema || result.Status != "completed" || result.Answer != "done" || result.Session == "" || len(result.Sessions) != 1 || result.Sessions[0] != result.Session || result.Pending != nil || len(result.Calls) != 0 || result.Artifacts == nil {
		t.Fatalf("%+v", result)
	}
	if result.Usage.Requests != 1 || result.Usage.PromptTokens != 12 || result.Usage.CompletionTokens != 3 || result.Usage.CostUSD != 0.0004 || result.Usage.CostRequests != 1 {
		t.Fatalf("usage %+v", result.Usage)
	}
}

// A run that stopped at a decision is decided without the console: --decide
// approve runs the call and the turn to its end in the same session, and
// the result counts both runs' calls. --decide deny tells the model no.
func TestExecDecidesThePendingCallOfASession(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	workspace := t.TempDir()
	scriptedModel(t, execChunk, answerChunk)
	code, stdout, _ := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file", "--json")
	first := execResult(t, stdout)
	if code != execNeedsDecision || first.Status != "needs_decision" || first.Pending == nil || first.Pending.Tool != "local_exec" || first.Pending.ID != "exec-1" || !strings.Contains(string(first.Pending.Arguments), "why") {
		t.Fatalf("code=%d %+v", code, first)
	}
	code, _, stderr := execRun(t, env, "--root", workspace, "--session", first.Session, "--decide", "approve", "--call", "other")
	if code != execFailed || !strings.Contains(stderr, "not other") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	calls := scriptedModel(t, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--session", first.Session, "--decide", "approve", "--call", "exec-1", "--json")
	if code != execCompleted || *calls != 1 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, *calls, stderr)
	}
	if _, err := os.Stat(filepath.Join(workspace, "made-by-exec")); err != nil {
		t.Fatalf("the approved command did not run: %v", err)
	}
	second := execResult(t, stdout)
	if second.Status != "completed" || second.Session != first.Session || second.Answer != "done" || len(second.Calls) != 1 || second.Calls[0].Status != "completed" || second.Calls[0].Decision != "approve" {
		t.Fatalf("%+v", second)
	}
	code, _, stderr = execRun(t, env, "--root", workspace, "--session", first.Session, "--decide", "approve")
	if code != execFailed || !strings.Contains(stderr, "no pending call") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	scriptedModel(t, execChunk, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "make the file", "--json")
	if code != execNeedsDecision {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	denied := execResult(t, stdout)
	scriptedModel(t, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--session", denied.Session, "--decide", "deny", "--json")
	result := execResult(t, stdout)
	if code != execCompleted || result.Status != "completed" || len(result.Calls) != 1 || result.Calls[0].Status != "denied" {
		t.Fatalf("code=%d stderr=%s %+v", code, stderr, result)
	}
}

func TestExecRefusesItsFlagsAlone(t *testing.T) {
	for _, args := range [][]string{
		{"--json"},
		{"--approve", "local"},
		{"--exec", "x", "--approve", "maybe"},
		{"--exec", "x", "--mode", "debug"},
		{"--exec", "x", "--repair", "it breaks"},
		{"--decide", "approve"},
		{"--session", "0123456789abcdef0123456789abcdef", "--decide", "maybe"},
		{"--session", "0123456789abcdef0123456789abcdef", "--decide", "approve", "--exec", "x"},
		{"--call", "c1"},
	} {
		env := cliEnv(t)
		code, _, stderr := execRun(t, env, append([]string{"--root", t.TempDir(), "--model", "test/model"}, args...)...)
		if code == 0 || stderr == "" {
			t.Fatalf("%v accepted: %s", args, stderr)
		}
	}
}

// A write without a why runs when --approve lets it run without a card,
// and the next request tells the model to give one; with a card due, it is
// refused as before (22 October 2026: each refusal made Claude Haiku 5.5
// send the whole file again).
func TestExecRunsAnUnapprovedWhylessWriteOnlyWithoutACard(t *testing.T) {
	const writeChunk = `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"write-1","type":"function","function":{"name":"local_write","arguments":"{\"path\":\"page.html\",\"content\":\"<p>hi</p>\",\"mode\":\"create\"}"}}]},"finish_reason":"tool_calls"}]}`
	for name, tc := range map[string]struct {
		approve  []string
		written  bool
		reminder bool
	}{
		"without a card":  {approve: []string{"--approve", "local"}, written: true, reminder: true},
		"with a card due": {written: false},
	} {
		t.Run(name, func(t *testing.T) {
			env := cliEnv(t)
			workspace := t.TempDir()
			previous := http.DefaultTransport
			defer func() { http.DefaultTransport = previous }()
			var bodies []string
			http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				data, _ := io.ReadAll(r.Body)
				bodies = append(bodies, string(data))
				chunk := answerChunk
				if len(bodies) == 1 {
					chunk = writeChunk
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
			})
			code, _, stderr := execRun(t, env, append([]string{"--root", workspace, "--model", "test/model", "--exec", "write the page"}, tc.approve...)...)
			if code != execCompleted {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
			_, err := os.Stat(filepath.Join(workspace, "page.html"))
			if (err == nil) != tc.written {
				t.Fatalf("written=%v, want %v", err == nil, tc.written)
			}
			if len(bodies) != 2 {
				t.Fatalf("%d requests", len(bodies))
			}
			if got := strings.Contains(bodies[1], "this call had no why"); got != tc.reminder {
				t.Fatalf("reminder=%v in %s", got, bodies[1])
			}
			if !tc.written && !strings.Contains(bodies[1], "give why") {
				t.Fatalf("the refusal does not ask for a why: %s", bodies[1])
			}
			if !strings.Contains(bodies[0], `"properties":{"why":`) {
				t.Fatal("why is not the first property of the schemas")
			}
		})
	}
}
