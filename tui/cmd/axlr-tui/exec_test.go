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
// program every runner has: touch, or cmd's mkdir on Windows. (cmd keeps the
// quotes Go puts around an argument with a space and a ">", and ran
// "type nul > made-by-exec" as one program name.)
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
	if joined := strings.Join(types, " "); !strings.HasPrefix(joined, "session") || !strings.Contains(joined, "tool.completed") || !strings.HasSuffix(joined, "completed") {
		t.Fatalf("events %s", joined)
	}
}

func TestExecRefusesItsFlagsAlone(t *testing.T) {
	for _, args := range [][]string{
		{"--json"},
		{"--approve", "local"},
		{"--exec", "x", "--approve", "maybe"},
		{"--exec", "x", "--mode", "debug"},
		{"--exec", "x", "--repair", "it breaks"},
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
