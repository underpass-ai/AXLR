package service

import (
	"context"
	"net/http"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// handleCloseSession ends the session's MADE instance, as the console does
// when the person leaves a session or exits. The transcript stays and a
// later turn continues in the instance's successor. It refuses while a
// turn runs: cancel it first.
func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	var input struct {
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	id := r.PathValue("id")
	if _, err := domain.NewSessionID(id); err != nil {
		writeError(w, requestID(r), 400, "invalid_id", "invalid session ID")
		return
	}
	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()
	session, ok := s.ownedSession(w, r)
	if !ok {
		return
	}
	if session.Export().Revision != input.ExpectedRevision {
		writeError(w, requestID(r), 409, "stale_revision", "session revision changed")
		return
	}
	s.mu.Lock()
	_, active := s.operations[id]
	s.mu.Unlock()
	if active || session.Status() == domain.StatusStreaming || len(session.Pending()) != 0 {
		writeError(w, requestID(r), 409, "operation_in_progress", "session has a running turn; cancel it first")
		return
	}
	if s.deps.Lifecycle != nil {
		if err := s.deps.Lifecycle.Close(context.WithoutCancel(r.Context()), &session, "closed"); err != nil {
			writeError(w, requestID(r), 500, "storage_error", "unable to close session")
			return
		}
	}
	instance, _ := session.Instance()
	_, _ = s.events.Append(id, session.Export().OperationID, "session.closed", map[string]any{"instance": instance.Instance, "closed": instance.Closed})
	current, err := s.sessions.Load(r.Context(), domain.SessionID(id))
	if err != nil {
		current = session
	}
	writeJSON(w, 200, map[string]any{"session_id": id, "status": "closed", "revision": current.Export().Revision})
}

// closeInstances ends, at shutdown, every open instance of this service's
// sessions; the next start sweeps whatever a crash left open.
func (s *Server) closeInstances() {
	if s.deps.Lifecycle == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	summaries, err := s.sessions.List(ctx)
	if err != nil {
		return
	}
	for _, summary := range summaries {
		session, err := s.sessions.Load(ctx, summary.ID)
		if err != nil {
			continue
		}
		if instance, has := session.Instance(); has && instance.Instance != "" && !instance.Closed {
			_ = s.deps.Lifecycle.Close(ctx, &session, "shutdown")
		}
	}
}

// continueCapped follows the console past the context cap (decision D10):
// the turn stopped, the session's instance closed, and the prompt starts
// the successor's first turn, owned by the same principal. The capped
// session's journal names the successor and the operation to follow.
func (s *Server) continueCapped(capped *application.ContextCapError, op, actor, request string) {
	successor := string(capped.Successor.ID())
	next, err := newID()
	if err != nil {
		_, _ = s.events.Append(string(capped.Predecessor), op, "operation.failed", map[string]string{"code": "operation_failed"})
		return
	}
	_, _ = s.events.Append(string(capped.Predecessor), op, "session.continued", map[string]any{"successor_id": successor, "operation_id": next, "tokens": capped.Tokens, "cap": capped.Cap})
	prompt, err := root.NewText(capped.Prompt)
	if err != nil || prompt == "" {
		return
	}
	if _, err := s.events.Append(successor, next, "operation.accepted", map[string]any{"status": "accepted", "predecessor_id": capped.Predecessor}); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.root)
	s.registerOperation(successor, cancel)
	if !s.startBackground(func() { s.runTurn(ctx, successor, next, actor, request, prompt) }) {
		cancel()
		s.finishOperation(successor)
	}
}
