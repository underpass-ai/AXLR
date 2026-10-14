package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/kmpstore"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A checkout's store is its own .kernel, pinned into the kmp registration
// before the plugin manager reads it; the installer's old per-user pin is
// replaced, and the bundle's about becomes the workspace's.
func TestPrepareKMPStorePinsTheCheckoutsStoreIntoTheRegistration(t *testing.T) {
	top := filepath.Join(t.TempDir(), "AXLR")
	workspace := filepath.Join(top, "tui")
	for _, dir := range []string{filepath.Join(top, ".git"), filepath.Join(top, ".kmp"), workspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(top, ".kmp", "memory.jsonl"), []byte(`{"abouts":["project:AXLR"]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(t.TempDir(), "share")
	user := kmpstore.UserDefault(data)
	// A bare binary that is not executable: the import is attempted and
	// fails, which must not stop the console.
	command := filepath.Join(t.TempDir(), "kmp-mcp")
	if err := os.WriteFile(command, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: command, AllowAll: true}, []string{"HOME=/h", kmpstore.DataDirVariable + "=" + user})
	if err != nil {
		t.Fatal(err)
	}
	other, err := plugins.NewRegistration(plugins.Manifest{ID: "made", Command: command, AllowAll: true}, []string{"HOME=/h"})
	if err != nil {
		t.Fatal(err)
	}
	registrations := []plugins.Registration{other, registration}
	store, found, err := prepareKMPStore(context.Background(), registrations, workspace, data)
	if !found || err != nil || !strings.Contains(store.Report.ImportFailed, "not executable") {
		t.Fatalf("found %v err %v report %+v", found, err, store.Report)
	}
	if store.About != "project:AXLR" || !store.FromBundle || store.bundleAbout() != "project:AXLR" {
		t.Fatalf("about %+v", store)
	}
	want := filepath.Join(top, kmpstore.StoreDirectory)
	if store.Selection.Path != want || store.Selection.Reason != kmpstore.ReasonProject || store.Selection.Replaced != user {
		t.Fatalf("selection %+v", store.Selection)
	}
	if got := kmpstore.Value(registrations[1].Env, kmpstore.DataDirVariable); got != want {
		t.Fatalf("pinned %q, want %q", got, want)
	}
	if kmpstore.Value(registrations[1].Env, kmpstore.BackendVariable) != "embedded" || len(registrations[0].Env) != 1 {
		t.Fatalf("environments %v %v", registrations[0].Env, registrations[1].Env)
	}
	if !store.Report.Created {
		t.Fatalf("report %+v", store.Report)
	}
	line := store.line(data)
	for _, part := range []string{"KMP store " + want, "the checkout " + top, "replaces the installer's pin " + user, "new store", ".kmp/memory.jsonl not imported: "} {
		if !strings.Contains(line, part) {
			t.Fatalf("line %q lacks %q", line, part)
		}
	}
}

func TestPrepareKMPStoreOutsideACheckoutUsesTheUserDefaultAndTheDirectoryAbout(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "My Notes")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(t.TempDir(), "share")
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: filepath.Join(t.TempDir(), "scripts", kmpstore.Launcher), AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registrations := []plugins.Registration{registration}
	store, found, err := prepareKMPStore(context.Background(), registrations, workspace, data)
	if !found || err != nil {
		t.Fatalf("found %v err %v", found, err)
	}
	if store.Selection.Reason != kmpstore.ReasonUser || store.Selection.Path != kmpstore.UserDefault(data) || store.About != "project:My-Notes" || store.FromBundle {
		t.Fatalf("%+v", store)
	}
	if !strings.Contains(store.line(data), "the per-user default, outside a checkout") || !strings.Contains(store.line(data), "about project:My-Notes") {
		t.Fatalf("line %q", store.line(data))
	}
	if _, found, _ := prepareKMPStore(context.Background(), nil, workspace, data); found {
		t.Fatal("no kmp registration, nothing to prepare")
	}
}

func TestKMPStoreLineDescribesWhatANewStoreReceived(t *testing.T) {
	store := kmpStore{Selection: kmpstore.Selection{Path: "/m", Reason: kmpstore.ReasonOperator}, Report: kmpstore.Report{Created: true, Imported: 7, Configured: []string{"typesafe.json"}, GuideSynced: true}, About: "project:AXLR", FromBundle: true}
	line := store.line("/data")
	for _, part := range []string{"pinned in mcp.json", "about project:AXLR (from .kmp/memory.jsonl)", "7 events imported", "1 configuration files copied from " + filepath.Join("/data", "kmp", "default"), "guide synced"} {
		if !strings.Contains(line, part) {
			t.Fatalf("line %q lacks %q", line, part)
		}
	}
	store.Report.Imported = -1
	if line := store.line("/data"); !strings.Contains(line, ".kmp/memory.jsonl imported") {
		t.Fatalf("line %q", line)
	}
}

func TestSelectBundleAboutFillsOnlyAMissingAbout(t *testing.T) {
	labels, err := storage.NewSessionLabelStore(filepath.Join(t.TempDir(), "labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	id := domain.SessionID(strings.Repeat("a", 32))
	if err := selectBundleAbout(context.Background(), labels, id, "project:AXLR"); err != nil {
		t.Fatal(err)
	}
	all, err := labels.Load(context.Background())
	if err != nil || all[id].About != "project:AXLR" {
		t.Fatalf("%+v %v", all[id], err)
	}
	if err := selectBundleAbout(context.Background(), labels, id, "project:Other"); err != nil {
		t.Fatal(err)
	}
	all, _ = labels.Load(context.Background())
	if all[id].About != "project:AXLR" {
		t.Fatalf("a selected about must be kept: %+v", all[id])
	}
}

// --root inside a checkout: the about is the checkout's name, bound to
// the session because the console's default derives from the workspace.
func TestPrepareKMPStoreInsideACheckoutUsesTheCheckoutsName(t *testing.T) {
	top := filepath.Join(t.TempDir(), "Repo-B")
	workspace := filepath.Join(top, "src", "deep")
	for _, dir := range []string{filepath.Join(top, ".git"), workspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: filepath.Join(t.TempDir(), "scripts", kmpstore.Launcher), AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, found, err := prepareKMPStore(context.Background(), []plugins.Registration{registration}, workspace, filepath.Join(t.TempDir(), "share"))
	if !found || err != nil {
		t.Fatalf("found %v err %v", found, err)
	}
	if store.About != "project:Repo-B" || !store.FromRoot || store.bundleAbout() != "project:Repo-B" || !strings.Contains(store.line("/d"), "about project:Repo-B (the checkout's name)") {
		t.Fatalf("%+v", store)
	}
}

func TestStartupLineSaysHowMuchTheStoreHolds(t *testing.T) {
	store := kmpStore{About: "project:X"}
	if store.memoriesLine() != "" {
		t.Fatal("an uncounted store says nothing")
	}
	store.Counted = true
	if store.memoriesLine() != "; new project, nothing recorded yet" {
		t.Fatal(store.memoriesLine())
	}
	store.Memories = 1
	if store.memoriesLine() != "; 1 memory for project:X" {
		t.Fatal(store.memoriesLine())
	}
	store.Memories = 12
	if store.memoriesLine() != "; 12 memories for project:X" {
		t.Fatal(store.memoriesLine())
	}
}
