package domain

import (
	"testing"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func TestSessionStampsEveryAddedMessageInUTCSeconds(t *testing.T) {
	at := time.Date(2026, 10, 1, 18, 2, 3, 900, time.FixedZone("CEST", 2*3600))
	defer func(previous func() time.Time) { now = previous }(now)
	now = func() time.Time { return at }
	s, err := NewSession("0123456789abcdef0123456789abcdef", "/w", "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("hola", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "hey"}}); err != nil {
		t.Fatal(err)
	}
	times := s.Export().MessageTimes
	want := time.Date(2026, 10, 1, 16, 2, 3, 0, time.UTC)
	if len(times) != 2 || !times[0].Equal(want) || times[0].Location() != time.UTC || !times[1].Equal(want) {
		t.Fatalf("times = %v", times)
	}
}

func TestOldSessionsWithoutTimesGetUnknownEntriesPadded(t *testing.T) {
	s, err := RestoreSession(SessionState{ID: "0123456789abcdef0123456789abcdef", Workspace: "/w", Model: "test/model", Status: StatusComplete, Messages: []axlr.Message{{Role: axlr.RoleUser, Content: "old"}, {Role: axlr.RoleAssistant, Content: "answer"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("new", nil); err != nil {
		t.Fatal(err)
	}
	times := s.Export().MessageTimes
	if len(times) != 3 || !times[0].IsZero() || !times[1].IsZero() || times[2].IsZero() {
		t.Fatalf("times = %v", times)
	}
}
