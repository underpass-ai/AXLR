package storage

import (
	"reflect"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A bundle carries the snapshot, the message times, the mode, the
// instance and the console notes, and comes back equal; the driven
// ceremony stays behind.
func TestSessionBundleRoundTrip(t *testing.T) {
	id, _ := domain.NewSessionID("0123456789abcdef0123456789abcdef")
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), root.ModelID("test/model"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("hola", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeReview); err != nil {
		t.Fatal(err)
	}
	instance := domain.SessionInstance{Instance: "axlr-session-" + string(id), Turns: 1, Closed: true, LastRecord: `{"turn":1}`, Pending: []string{`{"About":"ws:x"}`}, MemoryAgent: "a", MemoryContext: "c", Predecessor: "fedcba9876543210fedcba9876543210"}
	if err := s.SetInstance(instance); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseCeremony("axlr_notes", "1.0"); err != nil {
		t.Fatal(err)
	}
	meta := application.SessionBundleMeta{ExportedAt: time.Date(2026, 10, 13, 21, 0, 0, 0, time.UTC), Host: "laptop", Title: "Hola", About: "project:X"}
	data, err := SessionBundles{}.Encode(s, meta)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"version": 1`, `"host": "laptop"`, `"title": "Hola"`, `"about": "project:X"`, `"mode": "ceremony"`, `"instance": "axlr-session-`, `"chosen_ceremony": "axlr_notes"`} {
		if !strings.Contains(string(data), part) {
			t.Fatalf("bundle lacks %s:\n%s", part, data)
		}
	}
	back, got, err := SessionBundles{}.Decode(data)
	if err != nil || got != meta {
		t.Fatal(err, got)
	}
	state, want := back.Export(), s.Export()
	if state.ID != want.ID || state.Workspace != want.Workspace || len(state.Messages) != len(want.Messages) || back.Mode() != domain.ModeCeremony || state.ChosenCeremony != want.ChosenCeremony {
		t.Fatalf("state %+v", state)
	}
	if state.Instance == nil || state.Instance.Instance != instance.Instance || !state.Instance.Closed || state.Instance.MemoryContext != "c" || state.Instance.Predecessor != "fedcba9876543210fedcba9876543210" || len(state.Instance.Pending) != 1 {
		t.Fatalf("instance %+v", state.Instance)
	}
	if len(state.MessageTimes) != len(want.MessageTimes) {
		t.Fatalf("times %v want %v", state.MessageTimes, want.MessageTimes)
	}
	for _, bad := range []string{`{"version":2,"snapshot":{}}`, `{"version":1,"snapshot":{}} trailing`, `not json`} {
		if _, _, err := (SessionBundles{}).Decode([]byte(bad)); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

// A bundle of a session with tool calls imports: on 14 October 2026 the
// indented bundle laid the arguments out differently in the transcript and
// in the activity, and every such bundle was refused as inconsistent.
func TestSessionBundleWithToolCallsRoundTrips(t *testing.T) {
	s := fixture(t)
	schema, e := root.NewJSONObject([]byte(`{"type":"object","properties":{"path":{"type":"string"}}}`))
	must(t, e)
	identity, e := domain.NewLocalToolIdentity("read")
	must(t, e)
	args, e := root.NewJSONObject([]byte(`{"path":"a.go","note":"go test 2>&1 && echo <done>","nested":{"k":[1,2]}}`))
	must(t, e)
	must(t, s.BeginTurn("read", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "read", Parameters: schema}, Identity: identity}}))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: "read", Arguments: args}}}}))
	must(t, s.RecordToolOutcome("one", domain.DecisionApprove, domain.ToolOutcome{Content: "package a"}))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "read"}}))
	data, err := SessionBundles{}.Encode(s, application.SessionBundleMeta{})
	must(t, err)
	back, _, err := SessionBundles{}.Decode(data)
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, data)
	}
	if got, want := argumentBytes(back.Messages()), argumentBytes(s.Messages()); !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}
