package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/dto"
)

func invoke(t *testing.T, e *Executor, tool, args string) dto.Response {
	t.Helper()
	return e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "edge", Tool: tool, Arguments: json.RawMessage(args)})
}

func TestReadEmptyFileInvalidTextAndMissingPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty"), nil, 0600)
	os.WriteFile(filepath.Join(dir, "bad"), []byte{0xff}, 0600)
	os.Mkdir(filepath.Join(dir, "sub"), 0700)
	e, _ := New(Config{Root: dir})
	r := invoke(t, e, "read", `{"path":"empty"}`)
	if r.Status != "completed" || r.Output.(dto.ReadOutput).ContentSHA256 == "" {
		t.Fatalf("%+v", r)
	}
	for _, tc := range []struct{ path, code string }{{"bad", "invalid_text"}, {"missing", "not_found"}, {"sub", "not_regular_file"}} {
		r = invoke(t, e, "read", `{"path":"`+tc.path+`"}`)
		if r.Error == nil || r.Error.Code != tc.code {
			t.Fatalf("%s: %+v", tc.path, r)
		}
	}
	os.WriteFile(filepath.Join(dir, "multi"), []byte("é"), 0600)
	r = invoke(t, e, "read", `{"path":"multi","max_bytes":1}`)
	if r.Error == nil || r.Error.Code != "limit_too_small" {
		t.Fatalf("%+v", r)
	}
}

func TestWriteAndEditRejectInvalidPreconditionsWithoutMutating(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("alpha"), 0600)
	e, _ := New(Config{Root: dir, MaxFileBytes: 8})
	cases := []struct{ tool, args, code string }{
		{"write", `{"path":"a","content":"x","mode":"replace"}`, "invalid_arguments"},
		{"write", `{"path":"a","content":"x","mode":"create"}`, "conflict"},
		{"write", `{"path":"a","content":"123456789","mode":"replace","expected_sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`, "invalid_request"},
		{"edit", `{"path":"a","old_text":"absent","new_text":"x"}`, "conflict"},
		{"edit", `{"path":"a","old_text":"alpha","new_text":"123456789"}`, "file_too_large"},
		{"edit", `{"path":"a","old_text":"alpha","new_text":"x","expected_sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`, "conflict"},
	}
	for _, tc := range cases {
		r := invoke(t, e, tc.tool, tc.args)
		if r.Error == nil || r.Error.Code != tc.code {
			t.Fatalf("%s %s: %+v", tc.tool, tc.args, r)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a"))
	if string(b) != "alpha" {
		t.Fatalf("file changed to %q", b)
	}
}

func TestExecUsesOnlyConfiguredPATHAndRejectsEscapedCwd(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(dir, "link"))
	e, _ := New(Config{Root: dir, Env: []string{"PATH=/bin"}})
	r := invoke(t, e, "exec", `{"program":"sh","args":["-c","printf path-ok"]}`)
	if r.Status != "completed" || r.Output.(dto.ExecOutput).Stdout != "path-ok" {
		t.Fatalf("%+v", r)
	}
	r = invoke(t, e, "exec", `{"program":"missing-axlr-program"}`)
	if r.Error == nil || r.Error.Code != "start_failed" {
		t.Fatalf("%+v", r)
	}
	r = invoke(t, e, "exec", `{"program":"/bin/sh","cwd":"link"}`)
	if r.Error == nil || r.Error.Code != "invalid_path" {
		t.Fatalf("%+v", r)
	}
}

func TestSerializedResponseLimitIncludesEscapes(t *testing.T) {
	r := dto.Response{ProtocolVersion: 1, RequestID: "x", Tool: "read", Status: "completed", Output: dto.ReadOutput{Content: strings.Repeat("\x01", 1<<20)}}
	b, err := MarshalResponse(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > MaxResponseBytes {
		t.Fatalf("%d bytes", len(b))
	}
	var got dto.Response
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == nil || got.Error.Code != "response_too_large" {
		t.Fatalf("%+v", got)
	}
}

func TestHostProfileValidation(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []Config{{Root: dir, MaxTimeout: 6 * time.Minute}, {Root: dir, MaxReadBytes: 2 << 20}, {Root: dir, Env: []string{"BAD"}}, {Root: filepath.Join(dir, "missing")}} {
		if e, err := New(c); err == nil {
			e.Close()
			t.Fatalf("accepted %+v", c)
		}
	}
}

func TestDecodeRejectsMalformedUTF8InsteadOfReplacingIt(t *testing.T) {
	raw := append([]byte(`{"protocol_version":1,"request_id":"`), 0xff)
	raw = append(raw, []byte(`","tool":"read","arguments":{"path":"a"}}`)...)
	if _, err := Decode(strings.NewReader(string(raw))); err == nil {
		t.Fatal("accepted malformed UTF-8 request_id")
	}
}

func TestExecuteRejectsMalformedRawArgumentBeforeFileMutation(t *testing.T) {
	dir := t.TempDir()
	e, _ := New(Config{Root: dir})
	raw := append([]byte(`{"path":"a","mode":"create","content":"`), 0xff)
	raw = append(raw, []byte(`"}`)...)
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "bad", Tool: "write", Arguments: raw})
	if r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("file was created: %v", err)
	}
}

func TestDecodeRejectsLoneSurrogateEscapes(t *testing.T) {
	for _, raw := range []string{
		`{"protocol_version":1,"request_id":"\uD800","tool":"read","arguments":{"path":"a"}}`,
		`{"protocol_version":1,"request_id":"r","tool":"write","arguments":{"path":"a","mode":"create","content":"\uDC00"}}`,
	} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestEditRejectsOverlappingMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a")
	os.WriteFile(path, []byte("aaa"), 0600)
	e, _ := New(Config{Root: dir})
	r := invoke(t, e, "edit", `{"path":"a","old_text":"aa","new_text":"b"}`)
	if r.Error == nil || r.Error.Code != "conflict" {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "aaa" {
		t.Fatalf("file changed: %q", b)
	}
}

func TestReadRejectsFIFOWithoutBlocking(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Windows has no POSIX FIFO")
	}
	dir := t.TempDir()
	if err := makeFIFO(filepath.Join(dir, "pipe")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("pipe", filepath.Join(dir, "pipe-link")); err != nil {
		t.Fatal(err)
	}
	e, _ := New(Config{Root: dir})
	for _, name := range []string{"pipe", "pipe-link"} {
		done := make(chan dto.Response, 1)
		go func() { done <- invoke(t, e, "read", `{"path":"`+name+`"}`) }()
		select {
		case r := <-done:
			if r.Error == nil || r.Error.Code != "not_regular_file" {
				t.Fatalf("%s: %+v", name, r)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("read blocked on %s", name)
		}
	}
}

func TestMarshalResponseBoundsInvalidLibraryIdentity(t *testing.T) {
	r := dto.Response{ProtocolVersion: 1, RequestID: strings.Repeat("x", 5<<20), Tool: "read", Status: "rejected", Error: &dto.Failure{Code: "invalid_request", Message: "bad"}}
	b, err := MarshalResponse(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > MaxResponseBytes {
		t.Fatalf("response grew to %d bytes", len(b))
	}
}

func TestDecodeRejectsDuplicateAndCaseVariantFields(t *testing.T) {
	for _, raw := range []string{
		`{"protocol_version":1,"request_id":"x","request_id":"y","tool":"read","arguments":{"path":"a"}}`,
		`{"protocol_version":1,"Request_ID":"x","tool":"read","arguments":{"path":"a"}}`,
		`{"protocol_version":1,"request_id":"x","tool":"read","arguments":{"path":"a","path":"b"}}`,
	} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
