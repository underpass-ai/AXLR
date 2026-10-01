package service

import (
	"context"
	"net/http"
	"sync"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type pausedLoadStore struct {
	*storage.SessionStore
	once     sync.Once
	captured chan struct{}
	release  chan struct{}
}

func (s *pausedLoadStore) Load(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	session, err := s.SessionStore.Load(ctx, id)
	s.once.Do(func() {
		close(s.captured)
		<-s.release
	})
	return session, err
}

func TestCancelCannotOverwriteCompletedTurn(t *testing.T) {
	s, ts, client, _ := testServer(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(s.Config.Workspace), root.ModelID("test/model"))
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
	paused := &pausedLoadStore{SessionStore: s.base, captured: make(chan struct{}), release: make(chan struct{})}
	s.sessions.next = paused
	responses := make(chan *http.Response, 1)
	go func() {
		responses <- apiRequest(t, client, http.MethodPost, ts.URL+"/v1/sessions/"+string(id)+"/cancel", `{"expected_revision":2}`, "")
	}()
	<-paused.captured
	completed, err := s.base.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := completed.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "finished"}}); err != nil {
		t.Fatal(err)
	}
	completed.SetServiceMetadata("alice", 3, "operation-1")
	if err := s.base.Save(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	close(paused.release)
	response := <-responses
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("stale cancel status: %d", response.StatusCode)
	}
	stored, err := s.base.Load(context.Background(), id)
	if err != nil || stored.Status() != domain.StatusComplete || stored.Export().Revision != 3 {
		t.Fatalf("completed turn was overwritten: %+v, %v", stored.Export(), err)
	}
}
