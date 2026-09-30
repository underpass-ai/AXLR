package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	var input struct {
		Model string `json:"model"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	model, err := root.NewModelID(input.Model)
	if err != nil {
		writeError(w, requestID(r), 422, "invalid_model", "invalid model")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, requestID(r), 500, "internal_error", "unable to create session")
		return
	}
	session, err := domain.NewSession(domain.SessionID(id), domain.Workspace(s.Config.Workspace), model)
	if err != nil {
		writeError(w, requestID(r), 500, "internal_error", "unable to create session")
		return
	}
	session.SetServiceMetadata(principal(r).ID, 0, "")
	if err := s.sessions.Save(r.Context(), session); err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save session")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "model": model, "status": domain.StatusIdle, "revision": 1})
}

func (s *Server) ownedSession(w http.ResponseWriter, r *http.Request) (domain.Session, bool) {
	session, err := s.loadSession(r.Context(), r.PathValue("id"))
	if err != nil {
		if _, bad := domain.NewSessionID(r.PathValue("id")); bad != nil {
			writeError(w, requestID(r), 400, "invalid_id", "invalid session ID")
		} else if isNotFound(err) {
			writeError(w, requestID(r), 404, "not_found", "session not found")
		} else {
			writeError(w, requestID(r), 500, "storage_error", "unable to load session")
		}
		return domain.Session{}, false
	}
	if session.Export().Owner != principal(r).ID && !principal(r).Can("admin") {
		writeError(w, requestID(r), 403, "forbidden", "session belongs to another principal")
		return domain.Session{}, false
	}
	return session, true
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	session, ok := s.ownedSession(w, r)
	if !ok {
		return
	}
	state := session.Export()
	last, _, err := s.events.Read(string(state.ID), 0)
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to read events")
		return
	}
	writeJSON(w, 200, map[string]any{"id": state.ID, "model": state.Model, "status": state.Status, "revision": state.Revision, "operation_id": state.OperationID, "last_sequence": len(last), "messages": state.Messages, "pending": session.Pending()})
}

func (s *Server) handleStartTurn(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	key, ok := requireKey(w, r)
	if !ok {
		return
	}
	var input struct {
		Prompt           string `json:"prompt"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	prompt, err := root.NewText(input.Prompt)
	if err != nil || prompt == "" {
		writeError(w, requestID(r), 422, "invalid_prompt", "prompt is required")
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
	state := session.Export()
	body, _ := json.Marshal(input)
	if resource, duplicate, err := s.keys.Lookup(principal(r).ID, key, r.Method, r.URL.Path, body); err != nil {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	} else if duplicate {
		writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted", "events_url": "/v1/sessions/" + id + "/events"})
		return
	}
	if input.ExpectedRevision != state.Revision {
		writeError(w, requestID(r), 409, "stale_revision", "session revision changed")
		return
	}
	if session.Status() != domain.StatusIdle && session.Status() != domain.StatusComplete && session.Status() != domain.StatusInterrupted || len(session.Pending()) != 0 {
		writeError(w, requestID(r), 409, "invalid_state", "session is not ready for a turn")
		return
	}
	s.mu.Lock()
	_, active := s.operations[id]
	s.mu.Unlock()
	if active {
		writeError(w, requestID(r), 409, "operation_in_progress", "session already has an operation")
		return
	}
	op, err := newID()
	if err != nil {
		writeError(w, requestID(r), 500, "internal_error", "unable to start operation")
		return
	}
	resource, duplicate, err := s.keys.Claim(principal(r).ID, key, r.Method, r.URL.Path, body, op)
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for a different request")
		return
	}
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save operation")
		return
	}
	if duplicate {
		writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted", "events_url": "/v1/sessions/" + id + "/events"})
		return
	}
	ctx, cancel := context.WithCancel(s.root)
	s.registerOperation(id, cancel)
	if !s.startBackground(func() { s.runTurn(ctx, id, resource, prompt) }) {
		cancel()
		s.finishOperation(id)
		writeError(w, requestID(r), 503, "server_stopping", "server is stopping")
		return
	}
	writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted", "events_url": "/v1/sessions/" + id + "/events"})
}

func (s *Server) runTurn(ctx context.Context, id, op string, prompt root.Text) {
	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()
	defer s.finishOperation(id)
	session, err := s.loadSession(ctx, id)
	if err != nil {
		_, _ = s.events.Append(id, op, "operation.failed", map[string]string{"code": "storage_error"})
		return
	}
	session.SetServiceMetadata(session.Export().Owner, session.Export().Revision, op)
	err = s.deps.Start.Execute(ctx, &session, prompt, func(event application.Event) error { return s.recordApplicationEvent(id, op, event) })
	if err != nil {
		_, _ = s.events.Append(id, op, "operation.failed", map[string]string{"code": "operation_failed"})
	}
}

func (s *Server) recordApplicationEvent(id, op string, e application.Event) error {
	kind := ""
	var payload any = map[string]any{}
	switch e.Kind {
	case application.EventStreamStart:
		kind = "turn.started"
		payload = map[string]any{"status": "running"}
	case application.EventTextDelta:
		kind = "text.delta"
		payload = map[string]any{"text": e.Text}
	case application.EventToolActivity:
		if e.Tool.Outcome == nil {
			kind = "tool.requested"
		} else {
			kind = "tool.completed"
		}
		payload = map[string]any{"call_id": e.Tool.Call.ID, "tool": e.Tool.Call.Name, "decision": e.Tool.Decision, "outcome": e.Tool.Outcome}
	case application.EventState:
		switch e.State {
		case domain.StatusApproval:
			kind = "approval.required"
		case domain.StatusComplete:
			kind = "turn.completed"
		case domain.StatusInterrupted:
			kind = "turn.interrupted"
		}
		payload = map[string]any{"status": e.State}
	}
	if kind == "" {
		return nil
	}
	_, err := s.events.Append(id, op, kind, payload)
	return err
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	select {
	case s.streamSlots <- struct{}{}:
		defer func() { <-s.streamSlots }()
	default:
		writeError(w, requestID(r), 429, "too_many_streams", "too many event streams")
		return
	}
	_, ok := s.ownedSession(w, r)
	if !ok {
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		writeError(w, requestID(r), 400, "invalid_accept", "text/event-stream is required")
		return
	}
	query := r.URL.Query().Get("after")
	if query == "" {
		query = r.Header.Get("Last-Event-ID")
	}
	var after uint64
	if query != "" {
		n, err := strconv.ParseUint(query, 10, 64)
		if err != nil {
			writeError(w, requestID(r), 400, "invalid_cursor", "event cursor must be an integer")
			return
		}
		after = n
	}
	id := r.PathValue("id")
	events, changed, err := s.events.Read(id, after)
	if err != nil {
		writeError(w, requestID(r), 400, "invalid_cursor", "event cursor is invalid")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, requestID(r), 500, "stream_unavailable", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		for _, event := range events {
			data, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, data); err != nil {
				return
			}
			after = event.Sequence
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-changed:
			events, changed, err = s.events.Read(id, after)
			if err != nil {
				return
			}
		}
	}
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "approver") {
		return
	}
	key, ok := requireKey(w, r)
	if !ok {
		return
	}
	var input struct {
		Decision         string `json:"decision"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if input.Decision != "approve" && input.Decision != "deny" {
		writeError(w, requestID(r), 422, "invalid_decision", "decision must be approve or deny")
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
	session, err := s.loadSession(r.Context(), id)
	if err != nil {
		writeError(w, requestID(r), 404, "not_found", "session not found")
		return
	}
	if session.Export().Owner != principal(r).ID && !principal(r).Can("admin") && !principal(r).Can("approver") {
		writeError(w, requestID(r), 403, "forbidden", "approval is not authorized")
		return
	}
	body, _ := json.Marshal(input)
	if resource, duplicate, err := s.keys.Lookup(principal(r).ID, key, r.Method, r.URL.Path, body); err != nil {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	} else if duplicate {
		writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted"})
		return
	}
	if session.Export().Revision != input.ExpectedRevision {
		writeError(w, requestID(r), 409, "stale_revision", "session revision changed")
		return
	}
	pending := session.Pending()
	if len(pending) == 0 || string(pending[0].Call.ID) != r.PathValue("call_id") {
		writeError(w, requestID(r), 409, "invalid_state", "call is not pending")
		return
	}
	s.mu.Lock()
	_, active := s.operations[id]
	s.mu.Unlock()
	if active {
		writeError(w, requestID(r), 409, "operation_in_progress", "session already has an operation")
		return
	}
	op, err := newID()
	if err != nil {
		writeError(w, requestID(r), 500, "internal_error", "unable to create operation")
		return
	}
	resource, duplicate, err := s.keys.Claim(principal(r).ID, key, r.Method, r.URL.Path, body, op)
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	}
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save decision")
		return
	}
	if duplicate {
		writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted"})
		return
	}
	decision := domain.DecisionApprove
	if input.Decision == "deny" {
		decision = domain.DecisionDeny
	}
	ctx, cancel := context.WithCancel(s.root)
	s.registerOperation(id, cancel)
	if _, err := s.events.Append(id, resource, "approval.decision", map[string]any{"call_id": pending[0].Call.ID, "decision": input.Decision}); err != nil {
		cancel()
		s.finishOperation(id)
		writeError(w, requestID(r), 500, "storage_error", "unable to save decision event")
		return
	}
	if !s.startBackground(func() { s.runApproval(ctx, id, resource, pending[0].Call.ID, decision) }) {
		cancel()
		s.finishOperation(id)
		writeError(w, requestID(r), 503, "server_stopping", "server is stopping")
		return
	}
	writeJSON(w, 202, map[string]any{"operation_id": resource, "session_id": id, "status": "accepted"})
}

func (s *Server) runApproval(ctx context.Context, id, op string, call root.ToolCallID, decision domain.ToolDecision) {
	lock := s.sessionLock(id)
	lock.Lock()
	defer lock.Unlock()
	defer s.finishOperation(id)
	session, err := s.loadSession(ctx, id)
	if err != nil {
		_, _ = s.events.Append(id, op, "operation.failed", map[string]string{"code": "storage_error"})
		return
	}
	if session.Status() == domain.StatusInterrupted && len(session.Pending()) > 0 {
		if err := session.ResumePending(); err != nil {
			_, _ = s.events.Append(id, op, "operation.failed", map[string]string{"code": "operation_failed"})
			return
		}
	}
	session.SetServiceMetadata(session.Export().Owner, session.Export().Revision, op)
	err = s.deps.Resolve.Execute(ctx, &session, call, decision, func(event application.Event) error { return s.recordApplicationEvent(id, op, event) })
	if err != nil {
		_, _ = s.events.Append(id, op, "operation.failed", map[string]string{"code": "operation_failed"})
	}
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "session_client") {
		return
	}
	var input struct {
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	session, ok := s.ownedSession(w, r)
	if !ok {
		return
	}
	if session.Export().Revision != input.ExpectedRevision {
		writeError(w, requestID(r), 409, "stale_revision", "session revision changed")
		return
	}
	id := r.PathValue("id")
	if !s.cancelOperation(id) {
		lock := s.sessionLock(id)
		lock.Lock()
		defer lock.Unlock()
		if session.Status() != domain.StatusInterrupted && session.Status() != domain.StatusApproval && session.Status() != domain.StatusStreaming {
			writeError(w, requestID(r), 409, "invalid_state", "session has no active turn")
			return
		}
		if err := session.CancelPending(); err != nil || s.sessions.Save(r.Context(), session) != nil {
			writeError(w, requestID(r), 500, "storage_error", "unable to cancel turn")
			return
		}
		_, _ = s.events.Append(id, session.Export().OperationID, "turn.interrupted", map[string]any{"status": "interrupted"})
	}
	writeJSON(w, 202, map[string]any{"session_id": id, "status": "cancelling"})
}
