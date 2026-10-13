package domain

import "testing"

func TestSessionInstanceIsValidatedAndCopied(t *testing.T) {
	s, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if _, has := s.Instance(); has {
		t.Fatal("a new session has no instance")
	}
	if err := s.SetInstance(SessionInstance{}); err == nil {
		t.Fatal("an empty instance is refused")
	}
	if err := s.SetInstance(SessionInstance{Instance: "axlr-session-x", Turns: -1}); err == nil {
		t.Fatal("negative counters are refused")
	}
	if err := s.SetInstance(SessionInstance{Unavailable: "MADE not prepared"}); err != nil {
		t.Fatal(err)
	}
	instance := SessionInstance{Instance: "axlr-session-x", Turns: 2, Fence: "f", TurnIndex: 4, ClaimCost: 0.5}
	if err := s.SetInstance(instance); err != nil {
		t.Fatal(err)
	}
	instance.Fence = "changed"
	got, has := s.Instance()
	if !has || got.Fence != "f" || got.Turns != 2 || got.Instance != "axlr-session-x" {
		t.Fatalf("instance %+v", got)
	}
	exported := s.Export()
	exported.Instance.Fence = "mutated"
	if got, _ := s.Instance(); got.Fence != "f" {
		t.Fatal("Export must clone the instance")
	}
	restored, err := RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	if got, has := restored.Instance(); !has || got.TurnIndex != 4 {
		t.Fatalf("restored %+v", got)
	}
}
