package storage

import (
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
	s, err := domain.NewSession(id, domain.Workspace("/tmp/ws"), root.ModelID("test/model"))
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
	instance := domain.SessionInstance{Instance: "axlr-session-" + string(id), Turns: 1, Closed: true, LastRecord: `{"turn":1}`, Pending: []string{`{"About":"ws:x"}`}, KMPAgent: "a", KMPContext: "c", Predecessor: "p"}
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
	if state.Instance == nil || state.Instance.Instance != instance.Instance || !state.Instance.Closed || state.Instance.KMPContext != "c" || state.Instance.Predecessor != "p" || len(state.Instance.Pending) != 1 {
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
