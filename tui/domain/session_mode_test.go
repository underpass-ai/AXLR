package domain

import (
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func idleSession(t *testing.T) Session {
	t.Helper()
	s, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionModeDefaultsToNormalAndSurvivesRestore(t *testing.T) {
	s := idleSession(t)
	if s.Mode() != ModeNormal {
		t.Fatalf("new session mode %q", s.Mode())
	}
	if err := s.SetMode(ModeWriter); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil || restored.Mode() != ModeWriter {
		t.Fatalf("%v %q", err, restored.Mode())
	}
	state := s.Export()
	state.Mode = "loud"
	if _, err := RestoreSession(state); err == nil {
		t.Fatal("restored an unknown mode")
	}
}

func TestSessionModeChangesOnlyBetweenTurns(t *testing.T) {
	s := idleSession(t)
	if err := s.SetMode("loud"); err == nil {
		t.Fatal("accepted an unknown mode")
	}
	if err := s.BeginTurn(axlr.Text("hola"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(ModeReview); err == nil {
		t.Fatal("mode changed during a streaming turn")
	}
}

func TestSessionModeCarriesIntoTheNextTurn(t *testing.T) {
	s := idleSession(t)
	if err := s.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn(axlr.Text("revisa"), nil); err != nil {
		t.Fatal(err)
	}
	if s.Mode() != ModeReview {
		t.Fatalf("turn dropped the mode: %q", s.Mode())
	}
}

func TestChangeModelMessageDuringStreamingTurn(t *testing.T) {
	s := idleSession(t)
	if err := s.BeginTurn(axlr.Text("hola"), nil); err != nil {
		t.Fatal(err)
	}
	err := s.ChangeModel("other/model")
	if err == nil {
		t.Fatal("ChangeModel should reject during streaming turn")
	}
	if err.Error() != "cannot change model while turn is active" {
		t.Fatalf("wrong ChangeModel message: %q", err.Error())
	}
}
