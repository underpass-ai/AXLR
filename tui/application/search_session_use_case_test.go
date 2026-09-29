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
