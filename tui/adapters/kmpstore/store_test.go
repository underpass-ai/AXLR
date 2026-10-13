package kmpstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkout(t *testing.T) (top, workspace string) {
	t.Helper()
	top = filepath.Join(t.TempDir(), "AXLR")
	workspace = filepath.Join(top, "tui", "cmd")
	if err := os.MkdirAll(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	return top, workspace
}

func TestResolvePrefersTheOperatorThenTheCheckoutThenTheUserDefault(t *testing.T) {
	top, workspace := checkout(t)
	data := filepath.Join(t.TempDir(), "share")
	user := filepath.Join(data, "kmp", "default")
	operator := filepath.Join(t.TempDir(), "memory")
	if got := Resolve(workspace, []string{"HOME=/h", DataDirVariable + "=" + operator}, data); got != (Selection{Path: operator, Reason: ReasonOperator}) {
		t.Fatalf("operator: %+v", got)
	}
	// The installer's old pin of the per-user default is not an operator's choice.
	if got := Resolve(workspace, []string{DataDirVariable + "=" + user}, data); got != (Selection{Path: filepath.Join(top, StoreDirectory), Reason: ReasonProject, TopLevel: top, Replaced: user}) {
		t.Fatalf("replaced pin: %+v", got)
	}
	if got := Resolve(workspace, nil, data); got != (Selection{Path: filepath.Join(top, StoreDirectory), Reason: ReasonProject, TopLevel: top}) {
		t.Fatalf("project: %+v", got)
	}
	outside := t.TempDir()
	if got := Resolve(outside, []string{DataDirVariable + "=" + user}, data); got != (Selection{Path: user, Reason: ReasonUser}) {
		t.Fatalf("user: %+v", got)
	}
}

func TestGitTopLevelAcceptsAWorktreeFile(t *testing.T) {
	top := t.TempDir()
	if err := os.WriteFile(filepath.Join(top, ".git"), []byte("gitdir: /elsewhere/.git/worktrees/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(top, "a", "b")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := GitTopLevel(inner); !ok || got != top {
		t.Fatalf("%q %v", got, ok)
	}
	if _, ok := GitTopLevel(t.TempDir()); ok {
		t.Fatal("a directory without .git is not a checkout")
	}
}

func TestWithStorePinsTheStoreAndTheEmbeddedBackendOnce(t *testing.T) {
	got := WithStore([]string{"PATH=/bin", DataDirVariable + "=/old", "HOME=/h"}, "/new")
	want := []string{"HOME=/h", BackendVariable + "=embedded", DataDirVariable + "=/new", "PATH=/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	got = WithStore([]string{BackendVariable + "=grpc"}, "/new")
	if !reflect.DeepEqual(got, []string{BackendVariable + "=grpc", DataDirVariable + "=/new"}) {
		t.Fatalf("a set backend is kept: %v", got)
	}
}

func TestBinaryFindsTheEngineBesideThePackageLauncher(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "scripts", Launcher)
	if binary, plugin := Binary(launcher); binary != filepath.Join(root, "bin", "kmp-mcp") || plugin != "" {
		t.Fatalf("without guide assets: %q %q", binary, plugin)
	}
	if err := os.MkdirAll(filepath.Join(root, "guide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide", "guide.requests.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if binary, plugin := Binary(launcher); binary != filepath.Join(root, "bin", "kmp-mcp") || plugin != root {
		t.Fatalf("with guide assets: %q %q", binary, plugin)
	}
	bare := filepath.Join(root, "kmp-mcp")
	if binary, plugin := Binary(bare); binary != bare || plugin != "" {
		t.Fatalf("bare binary: %q %q", binary, plugin)
	}
}

type run struct {
	command string
	args    []string
	env     []string
}

func TestPrepareCreatesImportsConfiguresAndSyncsANewProjectStore(t *testing.T) {
	top, _ := checkout(t)
	if err := os.MkdirAll(filepath.Join(top, ".kmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(top, ".kmp", "memory.jsonl")
	if err := os.WriteFile(bundle, []byte(`{"abouts":["project:AXLR"],"event_count":7}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(t.TempDir(), "default")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"typesafe.json", "rerank.json", "agent-users.sqlite3"} {
		if err := os.WriteFile(filepath.Join(user, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var runs []run
	p := Preparer{Binary: "/opt/kmp/bin/kmp-mcp", PluginRoot: "/opt/kmp", UserStore: user, Run: func(_ context.Context, command string, args, env []string) ([]byte, error) {
		runs = append(runs, run{command, args, env})
		if args[0] == "import" {
			return []byte("note\n" + `{"events_imported":7,"mutations_applied":99}` + "\n"), nil
		}
		return nil, nil
	}}
	selection := Resolve(top, []string{"HOME=/h", "PATH=/bin"}, t.TempDir())
	report, err := p.Prepare(context.Background(), selection, []string{"HOME=/h", "PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Created || report.Imported != 7 || !report.GuideSynced || !reflect.DeepEqual(report.Configured, []string{"typesafe.json", "rerank.json"}) {
		t.Fatalf("%+v", report)
	}
	child := []string{"HOME=/h", BackendVariable + "=embedded", DataDirVariable + "=" + selection.Path, "PATH=/bin"}
	want := []run{{"/opt/kmp/bin/kmp-mcp", []string{"import", bundle}, child}, {"/opt/kmp/bin/kmp-mcp", []string{"guide", "sync", "--plugin-root", "/opt/kmp"}, child}}
	if !reflect.DeepEqual(runs, want) {
		t.Fatalf("runs %+v", runs)
	}
	if data, err := os.ReadFile(filepath.Join(selection.Path, ".gitignore")); err != nil || string(data) != "*\n" {
		t.Fatalf("gitignore %q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(selection.Path, "typesafe.json")); err != nil || string(data) != "typesafe.json" {
		t.Fatalf("configuration %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(selection.Path, "agent-users.sqlite3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("agents are not copied")
	}
}

func TestPrepareLeavesAnExistingStoreAlone(t *testing.T) {
	store := t.TempDir()
	if err := os.WriteFile(filepath.Join(store, "FORMAT_VERSION"), []byte("4"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Preparer{Binary: "/opt/kmp/bin/kmp-mcp", PluginRoot: "/opt/kmp", Run: func(context.Context, string, []string, []string) ([]byte, error) {
		t.Fatal("nothing runs on an existing store")
		return nil, nil
	}}
	report, err := p.Prepare(context.Background(), Selection{Path: store, Reason: ReasonOperator}, nil)
	if err != nil || report.Created || report.Imported != 0 || report.GuideSynced || len(report.Configured) != 0 {
		t.Fatalf("%+v %v", report, err)
	}
}

func TestPrepareWithoutABinaryOrBundleOnlyCreatesTheDirectory(t *testing.T) {
	store := filepath.Join(t.TempDir(), "default")
	report, err := Preparer{}.Prepare(context.Background(), Selection{Path: store, Reason: ReasonUser}, nil)
	if err != nil || !report.Created || report.Imported != 0 || report.GuideSynced {
		t.Fatalf("%+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(store, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Preparer{}).Prepare(context.Background(), Selection{Path: "relative"}, nil); err == nil {
		t.Fatal("a relative path is refused")
	}
}

func TestPrepareReportsAFailedImport(t *testing.T) {
	top, _ := checkout(t)
	if err := os.MkdirAll(filepath.Join(top, ".kmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".kmp", "memory.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Preparer{Binary: "/opt/kmp/bin/kmp-mcp", Run: func(context.Context, string, []string, []string) ([]byte, error) {
		return nil, errors.New("store is not empty")
	}}
	_, err := p.Prepare(context.Background(), Resolve(top, nil, t.TempDir()), nil)
	if err == nil || !strings.Contains(err.Error(), "import") || !strings.Contains(err.Error(), "store is not empty") {
		t.Fatalf("%v", err)
	}
}

func TestImportedEventsReadsTheLastReceipt(t *testing.T) {
	if got := importedEvents([]byte("x\n{\"events_imported\":468}\n")); got != 468 {
		t.Fatal(got)
	}
	if got := importedEvents([]byte("no receipt")); got != -1 {
		t.Fatal(got)
	}
}

func TestBundleAboutTakesTheProjectAboutOrTheDirectoryMatch(t *testing.T) {
	top := filepath.Join(t.TempDir(), "AXLR")
	if about, err := BundleAbout(top); err != nil || about != "" {
		t.Fatalf("no bundle: %q %v", about, err)
	}
	if err := os.MkdirAll(filepath.Join(top, ".kmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(top, ".kmp", "memory.jsonl")
	write := func(header string) {
		if err := os.WriteFile(bundle, []byte(header+"\n{\"event\":1}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"abouts":["guide:kmp","project:Axlr"]}`)
	if about, err := BundleAbout(top); err != nil || about != "project:Axlr" {
		t.Fatalf("one project about: %q %v", about, err)
	}
	write(`{"abouts":["project:other","project:axlr"]}`)
	if about, err := BundleAbout(top); err != nil || about != "project:axlr" {
		t.Fatalf("directory match: %q %v", about, err)
	}
	write(`{"abouts":["project:other","project:another"]}`)
	if about, err := BundleAbout(top); err != nil || about != "" {
		t.Fatalf("no match: %q %v", about, err)
	}
	write(`not json`)
	if _, err := BundleAbout(top); err == nil {
		t.Fatal("an unreadable header is an error")
	}
	if about, err := BundleAbout(""); err != nil || about != "" {
		t.Fatalf("no top level: %q %v", about, err)
	}
}
