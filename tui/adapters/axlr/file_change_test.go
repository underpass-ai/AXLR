package axlr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestToolRunnerCapturesActualLocalWritesAndEdits(t *testing.T) {
	dir := t.TempDir()
	executor, err := runtime.New(runtime.Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	runner := ToolRunner{Executor: executor}
	run := func(op, input string) domain.ToolOutcome {
		t.Helper()
		id, _ := domain.NewLocalToolIdentity(op)
		out, err := runner.Execute(context.Background(), id, jsonValue(t, input))
		if err != nil || out.IsError {
			t.Fatalf("%s: %+v %v", op, out, err)
		}
		return out
	}
	out := run("write", `{"path":"sample.go","mode":"create","content":"package sample\n\nconst N = 1\n"}`)
	if out.Change == nil || !out.Change.Created || out.Change.Before != "" || out.Change.After != "package sample\n\nconst N = 1\n" {
		t.Fatalf("creation lost: %+v", out.Change)
	}
	if strings.Contains(string(out.Content), "before") || strings.Contains(string(out.Content), "change") {
		t.Fatal("review content leaked into model tool result")
	}
	out = run("edit", `{"path":"sample.go","old_text":"N = 1","new_text":"N = 2"}`)
	if out.Change == nil || out.Change.Created || out.Change.Before != "package sample\n\nconst N = 1\n" || out.Change.After != "package sample\n\nconst N = 2\n" {
		t.Fatalf("edit lost: %+v", out.Change)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sample.go"))
	if err != nil || string(data) != string(out.Change.After) {
		t.Fatalf("preview does not match actual file: %q %v", data, err)
	}
	input, _ := json.Marshal(dto.WriteArgs{Path: "sample.go", Mode: "replace", Content: "replacement", ExpectedSHA256: string(root.DigestOf(data))})
	out = run("write", string(input))
	if out.Change == nil || out.Change.Before != "package sample\n\nconst N = 2\n" || out.Change.After != "replacement" {
		t.Fatalf("replacement lost: %+v", out.Change)
	}
	out = run("edit", `{"path":"sample.go","old_text":"replacement","new_text":"replacement"}`)
	if out.Change != nil {
		t.Fatal("no-op edit was presented as a change")
	}
	// The test binary with no tests selected exits 0 on every platform.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	execInput, _ := json.Marshal(dto.ExecArgs{Program: self, Args: []string{"-test.run=^$"}})
	for _, op := range []string{"read", "exec"} {
		input := `{"path":"sample.go"}`
		if op == "exec" {
			input = string(execInput)
		}
		if out := run(op, input); out.Change != nil {
			t.Fatalf("%s invented change evidence", op)
		}
	}
}

func TestToolRunnerPreviewFailureDoesNotBlockEffectsOrInventDiffs(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		limit               int
		want                string
	}{
		{"large before", strings.Repeat("x", domain.MaxChangePreviewBytes+1), "small", 0, "too_large"},
		{"large after", "small", strings.Repeat("x", domain.MaxChangePreviewBytes+1), 0, "too_large"},
		{"many lines", strings.Repeat("x\n", domain.MaxChangePreviewLines), "small", 0, "too_large"},
		{"binary before", "a\x00b", "text", 0, "unavailable"},
		{"read profile too small", "before", "after", 1, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "file"), []byte(tc.before), 0600); err != nil {
				t.Fatal(err)
			}
			e, err := runtime.New(runtime.Config{Root: dir, MaxReadBytes: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			args, _ := json.Marshal(dto.WriteArgs{Path: "file", Mode: "replace", Content: tc.after, ExpectedSHA256: string(root.DigestOf([]byte(tc.before)))})
			id, _ := domain.NewLocalToolIdentity("write")
			out, err := (ToolRunner{Executor: e}).Execute(context.Background(), id, jsonValue(t, string(args)))
			// AXLR refuses to replace binary input, so it must not create review evidence.
			if tc.name == "binary before" {
				if err != nil || !out.IsError || out.Change != nil {
					t.Fatalf("rejected binary write: %+v %v", out, err)
				}
				return
			}
			if err != nil || out.IsError || out.Change == nil || out.Change.Unavailable != tc.want || out.Change.Before != "" || out.Change.After != "" {
				t.Fatalf("bad bounded preview: %+v %v", out, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "file"))
			if err != nil || string(data) != tc.after {
				t.Fatalf("preview blocked write: %v", err)
			}
		})
	}
}

func TestRejectedOrCancelledWritesHaveNoReviewEvidence(t *testing.T) {
	e, err := runtime.New(runtime.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runner := ToolRunner{Executor: e}
	for _, op := range []string{"write", "edit"} {
		id, _ := domain.NewLocalToolIdentity(op)
		for _, input := range []string{`{"path":"missing","mode":"replace","content":"x"}`, `{"path":"../outside","mode":"create","content":"x"}`, `{"path":"missing","old_text":"x","new_text":"y"}`} {
			out, err := runner.Execute(context.Background(), id, jsonValue(t, input))
			if err != nil || !out.IsError || out.Change != nil {
				t.Fatalf("failed call invented a diff: %+v %v", out, err)
			}
		}
	}
	id, _ := domain.NewLocalToolIdentity("write")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := runner.Execute(ctx, id, jsonValue(t, `{"path":"new","mode":"create","content":"x"}`))
	if err != nil || !out.Uncertain || out.Change != nil {
		t.Fatalf("cancelled call: %+v %v", out, err)
	}
}

func TestChangeDigestMismatchNeverClaimsAnExactPreview(t *testing.T) {
	c := &domain.FileChange{Path: "file", Before: "before", After: "after"}
	got := completedChange(c, dto.Response{Status: "completed", Output: dto.WriteOutput{ContentSHA256: string(root.DigestOf([]byte("different effect")))}})
	if got == nil || got.Unavailable != "unavailable" || got.Before != "" || got.After != "" {
		t.Fatalf("unproven preview: %+v", got)
	}
}
