package service

import (
	"context"
	"os"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestLiveStatusAndRestartRecovery(t *testing.T) {
	cfg := validConfig(t.TempDir())
	if err := os.WriteFile(cfg.PrincipalsFile, []byte(`{"version":1,"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(cfg.Workspace), root.ModelID("test/model"))
	if err != nil {
		t.Fatal(err)
	}
	session.SetServiceMetadata("alice", 0, "")
	if err := s.sessions.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := session.BeginTurn("hello", nil); err != nil {
		t.Fatal(err)
	}
	session.SetServiceMetadata("alice", 1, "operation-1")
	if err := s.sessions.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	live, err := s.sessions.Load(context.Background(), id)
	if err != nil || live.Status() != domain.StatusStreaming {
		t.Fatalf("live state: %v, %v", live.Status(), err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	recovered, err := restarted.sessions.Load(context.Background(), id)
	if err != nil || recovered.Status() != domain.StatusInterrupted || recovered.Export().Revision != 3 {
		t.Fatalf("recovered state: %+v, %v", recovered.Export(), err)
	}
	events, _, err := restarted.events.Read(string(id), 0)
	if err != nil || len(events) != 1 || events[0].Type != "turn.interrupted" {
		t.Fatalf("recovery events: %+v, %v", events, err)
	}
}

func TestClaimWithoutResourceBecomesInterruptedOnRestart(t *testing.T) {
	cfg := validConfig(t.TempDir())
	if err := os.WriteFile(cfg.PrincipalsFile, []byte(`{"version":1,"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(cfg.Workspace), root.ModelID("test/model"))
	if err != nil {
		t.Fatal(err)
	}
	session.SetServiceMetadata("alice", 0, "")
	if err := s.sessions.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	op := "fedcba9876543210fedcba9876543210"
	if _, _, err := s.keys.Claim("alice", "0123456789abcdef", "POST", "/v1/sessions/"+string(id)+"/turns", []byte(`{"prompt":"hello"}`), op); err != nil {
		t.Fatal(err)
	}
	callID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err := s.keys.Claim("alice", "abcdef0123456789", "POST", "/v1/tool-calls", []byte(`{"tool":"read"}`), callID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	events, _, err := restarted.events.Read(string(id), 0)
	if err != nil || len(events) != 1 || events[0].OperationID != op || events[0].Type != "operation.failed" {
		t.Fatalf("missing failed intent: %+v, %v", events, err)
	}
	call, err := restarted.calls.Load(callID)
	if err != nil || call.Status != "cancelled" || call.Revision != 1 {
		t.Fatalf("missing call tombstone: %+v, %v", call, err)
	}
}
