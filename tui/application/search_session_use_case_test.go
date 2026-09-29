package application

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestSearchSessionCaseInsensitiveStableOrder(t *testing.T) {
	s := turnSession(t)
	if e := s.BeginTurn("Hello HELLO", nil); e != nil {
		t.Fatal(e)
	}
	if e := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "hello again"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.BeginTurn("unrelated", nil); e != nil {
		t.Fatal(e)
	}
	if e := s.InterruptDraft("HELLO draft"); e != nil {
		t.Fatal(e)
	}
	u := SearchSessionUseCase{}
	hits := u.Execute(s, "hElLo")
	if len(hits) != 3 || hits[0].MessageIndex == nil || *hits[0].MessageIndex != 0 || hits[1].MessageIndex == nil || *hits[1].MessageIndex != 1 || !hits[2].Draft || hits[2].MessageIndex != nil || hits[0].Content != "Hello HELLO" {
		t.Fatalf("bad order: %+v", hits)
	}
	if len(u.Execute(s, "")) != 0 || len(u.Execute(s, "absent")) != 0 {
		t.Fatal("spurious hits")
	}
}

func TestSearchIncludesArchivedDraftBetweenItsUserTurns(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("first match", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("archived match"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("second match", nil); err != nil {
		t.Fatal(err)
	}
	hits := (SearchSessionUseCase{}).Execute(s, "match")
	if len(hits) != 3 || hits[0].MessageIndex == nil || *hits[0].MessageIndex != 0 || hits[1].ArchivedDraftIndex == nil || *hits[1].ArchivedDraftIndex != 0 || hits[2].MessageIndex == nil || *hits[2].MessageIndex != 1 {
		t.Fatalf("archived draft search order is wrong: %+v", hits)
	}
}
