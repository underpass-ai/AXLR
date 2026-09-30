package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestCreateSessionPersistsBeforeReturn(t *testing.T) {
	store := &memoryStore{}
	s, err := (CreateSessionUseCase{Store: store}).Execute(context.Background(), "0123456789abcdef0123456789abcdef", "/workspace", "chosen/model")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.states) != 1 || store.states[0].Model != "chosen/model" || store.states[0].Status != domain.StatusIdle || !reflect.DeepEqual(store.states[0], s.Export()) {
		t.Fatalf("session was not durably created: %+v", s.Export())
	}
}

func TestCreateSessionRejectsInvalidAndSaveFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    domain.SessionID
		model root.ModelID
		fail  bool
	}{
		{"invalid ID", "bad", "model", false},
		{"invalid model", "0123456789abcdef0123456789abcdef", "", false},
		{"save failure", "0123456789abcdef0123456789abcdef", "model", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			if tc.fail {
				store.err = errors.New("disk failed")
			}
			s, err := (CreateSessionUseCase{Store: store}).Execute(context.Background(), tc.id, "/workspace", tc.model)
			if err == nil || !reflect.DeepEqual(s, domain.Session{}) || len(store.states) != 0 {
				t.Fatalf("created invalid or unsaved session: %+v, %v", s, err)
			}
		})
	}
}

func TestChangeSessionModelSavesBeforePublishing(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	if err := (ChangeSessionModelUseCase{Store: store}).Execute(context.Background(), &s, "next/model"); err != nil {
		t.Fatal(err)
	}
	if len(store.states) != 1 || store.states[0].Model != "next/model" || s.Export().Model != "next/model" || len(s.Messages()) != 0 {
		t.Fatalf("change was not persisted: %+v, saves %d", s.Export(), len(store.states))
	}
}

func TestChangeSessionModelSaveFailurePreservesActiveModel(t *testing.T) {
	s := turnSession(t)
	before := s.Export()
	boom := errors.New("disk failed")
	store := &memoryStore{err: boom}
	if err := (ChangeSessionModelUseCase{Store: store}).Execute(context.Background(), &s, "next/model"); !errors.Is(err, boom) {
		t.Fatalf("save error lost: %v", err)
	}
	if !reflect.DeepEqual(before, s.Export()) || len(store.states) != 0 {
		t.Fatal("unsaved model became active")
	}
}

func TestChangeSessionModelDoesNotResumeInterruptedTurn(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("hello", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("partial"); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	if err := (ChangeSessionModelUseCase{Store: store}).Execute(context.Background(), &s, "next/model"); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusInterrupted || s.Export().Draft != "partial" || len(s.Messages()) != 1 || len(store.states) != 1 {
		t.Fatalf("model change resumed or altered turn: %+v", s.Export())
	}
}
