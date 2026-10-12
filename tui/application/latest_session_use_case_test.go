package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type summaryStore struct {
	memoryStore
	summaries []domain.SessionSummary
}

func (s *summaryStore) List(context.Context) ([]domain.SessionSummary, error) {
	return s.summaries, nil
}

// axlr-tui -c resumes the workspace's most recent session that has
// messages, is not archived and is not a plan task's worker.
func TestLatestSessionIsTheWorkspacesMostRecentConversation(t *testing.T) {
	at := func(minute int) time.Time { return time.Date(2026, 10, 12, 10, minute, 0, 0, time.UTC) }
	id := func(c string) domain.SessionID {
		s := ""
		for range 32 {
			s += c
		}
		return domain.SessionID(s)
	}
	store := &summaryStore{summaries: []domain.SessionSummary{
		{ID: id("a"), Workspace: "/ws", MessageCount: 4, UpdatedAt: at(1)},
		{ID: id("b"), Workspace: "/ws", MessageCount: 2, UpdatedAt: at(2)},
		{ID: id("c"), Workspace: "/ws", MessageCount: 9, UpdatedAt: at(3), Mode: domain.ModeTask},
		{ID: id("d"), Workspace: "/ws", MessageCount: 0, UpdatedAt: at(4)},
		{ID: id("e"), Workspace: "/ws", MessageCount: 3, UpdatedAt: at(5)},
		{ID: id("f"), Workspace: "/other", MessageCount: 3, UpdatedAt: at(6)},
	}}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{id("e"): {Archived: true}}}
	got, err := LatestSessionUseCase{Store: store, Labels: labels}.Execute(context.Background(), "/ws")
	if err != nil || got != id("b") {
		t.Fatalf("latest %q %v", got, err)
	}
	if _, err := (LatestSessionUseCase{Store: store}).Execute(context.Background(), "/empty"); !errors.Is(err, ErrNoSessionToContinue) {
		t.Fatalf("empty workspace: %v", err)
	}
}

// A session created from the console gets a fresh ID each time.
func TestCreateSessionExecuteNewDrawsAnID(t *testing.T) {
	store := &memoryStore{}
	u := CreateSessionUseCase{Store: store}
	ws := domain.Workspace(t.TempDir())
	first, err := u.ExecuteNew(context.Background(), ws, "test/model")
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.ExecuteNew(context.Background(), ws, "test/model")
	if err != nil || first.Export().ID == second.Export().ID || len(store.states) != 2 {
		t.Fatalf("ids %q %q, saves %d, %v", first.Export().ID, second.Export().ID, len(store.states), err)
	}
}
