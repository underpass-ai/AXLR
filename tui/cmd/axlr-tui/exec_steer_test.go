package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

// bodiesModel answers with chunks in turn, as scriptedModel, and keeps
// every request's body.
func bodiesModel(t *testing.T, chunks ...string) *[]string {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	bodies := []string{}
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		chunk := chunks[min(len(bodies), len(chunks)-1)]
		bodies = append(bodies, string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	return &bodies
}

func TestTheSteerFileGivesNewCompleteLinesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steer.jsonl")
	steer := newExecSteerFile(path)
	if _, ok := steer.Take(); ok {
		t.Fatal("a file that does not exist yet has nothing")
	}
	var told []root.Text
	steer.taken = func(text root.Text) { told = append(told, text) }
	write := func(data string) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.WriteString(data)
		_ = file.Close()
	}
	write("{\"text\": \"check row F too\"}\nplain words\n\n{\"text\": \"\"}\nhalf a li")
	text, ok := steer.Take()
	if !ok || text != "check row F too\n\nplain words" {
		t.Fatalf("%q %v", text, ok)
	}
	if _, ok := steer.Take(); ok {
		t.Fatal("a message is taken once; half a line waits for its newline")
	}
	write("ne\n" + strings.Repeat("x", maxSteerMessageBytes+1) + "\n")
	if text, ok := steer.Take(); !ok || text != "half a line" {
		t.Fatalf("%q %v", text, ok)
	}
	steer.Restore("kept")
	if text, ok := steer.Take(); !ok || text != "kept" {
		t.Fatalf("%q %v", text, ok)
	}
	if len(told) != 3 {
		t.Fatalf("taken %d times", len(told))
	}
	var none *execSteerFile
	if _, ok := none.Take(); ok {
		t.Fatal("no steer file, nothing")
	}
}

// A message written while the run works reaches the model after the next
// tool step, and the run says so; one written after the last step gets one
// more turn before the run ends.
func TestExecTakesWhatThePersonWritesWhileItWorks(t *testing.T) {
	env := cliEnv(t)
	env["PATH"] = os.Getenv("PATH")
	steerPath := filepath.Join(t.TempDir(), "steer.jsonl")
	if err := os.WriteFile(steerPath, []byte("{\"text\": \"also check row F\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bodies := bodiesModel(t, execChunk, answerChunk)
	code, stdout, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "make the file", "--approve", "local", "--steer-file", steerPath, "--json")
	if code != execCompleted || len(*bodies) != 2 {
		t.Fatalf("code=%d requests=%d stderr=%s", code, len(*bodies), stderr)
	}
	if strings.Contains((*bodies)[0], "also check row F") || !strings.Contains((*bodies)[1], "also check row F") {
		t.Fatal("the message goes with the request after the tool step")
	}
	steered := 0
	for _, line := range execLines(t, stdout) {
		if line.Type == "steered" {
			steered++
			if line.Text != "also check row F" || line.Session == "" {
				t.Fatalf("%+v", line)
			}
		}
	}
	if steered != 1 {
		t.Fatalf("steered %d times", steered)
	}

	// Written before a run that only answers: one more turn for it.
	if err := os.WriteFile(steerPath, []byte("what about row G?\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bodies = bodiesModel(t, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "say done", "--steer-file", steerPath, "--json")
	if code != execCompleted || len(*bodies) != 2 || !strings.Contains((*bodies)[1], "what about row G?") {
		t.Fatalf("code=%d requests=%d stderr=%s", code, len(*bodies), stderr)
	}
	if result := execResult(t, stdout); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
	// Without --exec the flag is refused.
	if code, _, stderr := execRun(t, env, "--steer-file", steerPath); code == 0 || !strings.Contains(stderr, "--steer-file go with --exec") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}
