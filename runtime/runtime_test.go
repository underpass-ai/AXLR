package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/dto"
)

func newTestExecutor(t *testing.T, config Config) (*Executor, error) {
	t.Helper()
	executor, err := New(config)
	if err == nil {
		t.Cleanup(func() { _ = executor.Close() })
	}
	return executor, err
}

func TestDecodeRejectsUnknownFieldAndSecondDocument(t *testing.T) {
	for _, input := range []string{
		`{"protocol_version":1,"request_id":"r1","tool":"read","arguments":{"path":"a"},"surprise":true}`,
		`{"protocol_version":1,"request_id":"r1","tool":"read","arguments":{"path":"a","surprise":true}}`,
		`{"protocol_version":1,"request_id":"r1","tool":"read","arguments":{"path":"a"}} {}`,
	} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestDecodeRejectsOversizeAndInvalidIdentity(t *testing.T) {
	if _, err := Decode(strings.NewReader(strings.Repeat("x", 4<<20+1))); err == nil {
		t.Fatal("oversize accepted")
	}
	for _, input := range []string{
		`{"protocol_version":1,"request_id":"","tool":"read","arguments":{"path":"a"}}`,
		`{"protocol_version":1,"request_id":"r1","tool":"unknown","arguments":{}}`,
	} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestReadPaginatesUTF8AndFullDigest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aéz"), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := newTestExecutor(t, Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "read", Arguments: json.RawMessage(`{"path":"a.txt","max_bytes":2}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	first := r.Output.(dto.ReadOutput)
	if first.Content != "a" || first.NextOffsetBytes != 1 || !first.Truncated || first.ContentSHA256 != "" {
		t.Fatalf("%+v", first)
	}
	r = e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r2", Tool: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)})
	full := r.Output.(dto.ReadOutput)
	if full.Content != "aéz" || full.ContentSHA256 == "" {
		t.Fatalf("%+v", full)
	}
}

func TestWriteCreateDoesNotClobberAndReplaceChecksDigest(t *testing.T) {
	dir := t.TempDir()
	e, err := newTestExecutor(t, Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	run := func(args string) dto.Response {
		return e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "write", Arguments: json.RawMessage(args)})
	}
	if r := run(`{"path":"a.txt","content":"one","mode":"create"}`); r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	if r := run(`{"path":"a.txt","content":"two","mode":"create"}`); r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
	if r := run(`{"path":"a.txt","content":"two","mode":"replace","expected_sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`); r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
	b, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(b) != "one" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestEditRequiresExactlyOneMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a a"), 0600)
	e, err := newTestExecutor(t, Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	req := dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "edit", Arguments: json.RawMessage(`{"path":"a.txt","old_text":"a","new_text":"b"}`)}
	if r := e.Execute(context.Background(), req); r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(b) != "a a" {
		t.Fatalf("%q", b)
	}
}

func TestExecDistinguishesExitCodeFromStartFailure(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario")
	}
	e, err := newTestExecutor(t, Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","printf ok; exit 7"]}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	out := r.Output.(dto.ExecOutput)
	if out.ExitCode != 7 || out.Stdout != "ok" {
		t.Fatalf("%+v", out)
	}
	r = e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r2", Tool: "exec", Arguments: json.RawMessage(`{"program":"/no/such/program"}`)})
	if r.Status != "failed" {
		t.Fatalf("%+v", r)
	}
}

func TestExecKeepsTheResultWhenABackgroundChildHoldsOutput(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario")
	}
	e, err := newTestExecutor(t, Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "bg", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","echo started; sleep 30 &"],"timeout_ms":10000}`)})
	if r.Status != "completed" {
		t.Fatalf("program that exited 0 reported as %s: %+v", r.Status, r.Error)
	}
	out := r.Output.(dto.ExecOutput)
	if out.ExitCode != 0 || out.Stdout != "started\n" || !out.Truncated {
		t.Fatalf("%+v", out)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v for the background child", elapsed)
	}
}

func TestReadRejectsEscapedSymlinkAndSplitOffset(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600)
	os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link"))
	os.WriteFile(filepath.Join(dir, "utf8"), []byte("é"), 0600)
	e, _ := newTestExecutor(t, Config{Root: dir})
	for _, a := range []string{`{"path":"../secret"}`, `{"path":"link"}`, `{"path":"utf8","offset_bytes":1}`} {
		r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "read", Arguments: json.RawMessage(a)})
		if r.Status == "completed" {
			t.Fatalf("accepted %s: %+v", a, r)
		}
	}
}

func TestWriteReplaceKeepsOrdinaryPermissionsAndEditChangesOneMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a")
	if err := os.WriteFile(path, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	e, _ := newTestExecutor(t, Config{Root: dir})
	oldDigest := "cba06b5736faf67e54b07b561eae94395e774c517a7d910a54369e1263ccfbd4"
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "w", Tool: "write", Arguments: json.RawMessage(`{"path":"a","content":"new","mode":"replace","expected_sha256":"` + oldDigest + `"}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	info, _ := os.Stat(path)
	if goruntime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	r = e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "e", Tool: "edit", Arguments: json.RawMessage(`{"path":"a","old_text":"new","new_text":"done"}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "done" {
		t.Fatalf("%q", b)
	}
}

func TestExecTimeoutAndOutputLimit(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario")
	}
	e, _ := newTestExecutor(t, Config{Root: t.TempDir()})
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "x", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","printf 12345; printf abcde >&2"],"max_output_bytes":5}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	out := r.Output.(dto.ExecOutput)
	if out.CapturedBytes != 5 || out.DiscardedBytes != 5 || !out.Truncated {
		t.Fatalf("%+v", out)
	}
	r = e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "t", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","while :; do :; done"],"timeout_ms":20}`)})
	if r.Status != "timed_out" {
		t.Fatalf("%+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = e.Execute(ctx, dto.Request{ProtocolVersion: 1, RequestID: "c", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh"}`)})
	if r.Status != "cancelled" {
		t.Fatalf("%+v", r)
	}
}

func TestExecOutputLimitNeverSplitsACharacter(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario")
	}
	e, err := newTestExecutor(t, Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, script, stdout string
		limit, captured      int
		discarded            int64
	}{
		{"limit inside é", `printf 'a\303\251b'`, "a", 2, 1, 3},
		{"limit after é", `printf 'a\303\251b'`, "aé", 3, 3, 1},
		{"program ends inside a character", `printf 'a\303'`, "a�", 0, 2, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			args, _ := json.Marshal(dto.ExecArgs{Program: "/bin/sh", Args: []string{"-c", c.script}, MaxOutputBytes: c.limit})
			r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "u", Tool: "exec", Arguments: args})
			if r.Status != "completed" {
				t.Fatalf("%+v", r)
			}
			out := r.Output.(dto.ExecOutput)
			if out.Stdout != c.stdout || out.CapturedBytes != c.captured || out.DiscardedBytes != c.discarded || out.Truncated != (c.discarded > 0) {
				t.Fatalf("stdout=%q captured=%d discarded=%d truncated=%t", out.Stdout, out.CapturedBytes, out.DiscardedBytes, out.Truncated)
			}
		})
	}
}

func TestExecRejectsOverflowTimeout(t *testing.T) {
	e, _ := newTestExecutor(t, Config{Root: t.TempDir(), MaxTimeout: time.Minute})
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "x", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","timeout_ms":9223372036854775807}`)})
	if r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
}

func TestExecDefaultsRespectReducedHostProfile(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX shell scenario")
	}
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), MaxOutputBytes: 4, MaxTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "p", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","printf abcde"]}`)})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	out := r.Output.(dto.ExecOutput)
	if out.Stdout != "abcd" || out.DiscardedBytes != 1 {
		t.Fatalf("%+v", out)
	}
}

func TestExecPassesLiteralArgvAndStdinWithoutInheritedEnvironment(t *testing.T) {
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Env: []string{"AXLR_TEST_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(dto.ExecArgs{Program: os.Args[0], Args: []string{"-test.run=TestAXLRHelperProcess", "--", "echo", "a b", "$HOME"}, Stdin: "payload"})
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "h", Tool: "exec", Arguments: arguments})
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	out := r.Output.(dto.ExecOutput)
	if out.Stdout != "payload|a b|$HOME|" {
		t.Fatalf("%+v", out)
	}
}

func TestExecCancellationStopsRunningProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	e, err := newTestExecutor(t, Config{Root: dir, Env: []string{"AXLR_TEST_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(dto.ExecArgs{Program: os.Args[0], Args: []string{"-test.run=TestAXLRHelperProcess", "--", "hang", marker}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan dto.Response, 1)
	go func() {
		done <- e.Execute(ctx, dto.Request{ProtocolVersion: 1, RequestID: "c", Tool: "exec", Arguments: arguments})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("helper never started")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case r := <-done:
		if r.Status != "cancelled" {
			t.Fatalf("%+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not return")
	}
}

func TestAXLRHelperProcess(t *testing.T) {
	if os.Getenv("AXLR_TEST_HELPER") != "1" {
		return
	}
	i := -1
	for n, arg := range os.Args {
		if arg == "--" {
			i = n
			break
		}
	}
	if i < 0 || i+1 >= len(os.Args) {
		os.Exit(3)
	}
	switch os.Args[i+1] {
	case "echo":
		b, _ := io.ReadAll(os.Stdin)
		fmt.Printf("%s|%s|%s|%s", b, os.Args[i+2], os.Args[i+3], os.Getenv("HOME"))
		os.Exit(0)
	case "hang":
		if err := os.WriteFile(os.Args[i+2], []byte("yes"), 0600); err != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(5)
}
