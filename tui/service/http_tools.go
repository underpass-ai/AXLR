package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func (s *Server) availableTools(r *http.Request) ([]domain.AvailableTool, error) {
	if s.deps.Catalog == nil {
		return nil, errors.New("tool catalog unavailable")
	}
	return s.deps.Catalog.Snapshot(r.Context())
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "tool_operator") {
		return
	}
	tools, err := s.availableTools(r)
	if err != nil {
		writeError(w, requestID(r), 503, "engine_unavailable", "tool catalog is unavailable")
		return
	}
	origin := r.URL.Query().Get("origin")
	if origin != "" && origin != "local" && origin != "mcp" {
		writeError(w, requestID(r), 400, "invalid_filter", "origin must be local or mcp")
		return
	}
	items := make([]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Identity.Kind == domain.ToolKindHost {
			continue
		}
		kind := "local"
		if tool.Identity.Kind == domain.ToolKindPlugin {
			kind = "mcp"
		}
		if origin != "" && origin != kind {
			continue
		}
		items = append(items, map[string]any{"name": tool.Definition.Name, "origin": kind, "identity": tool.Identity, "description": tool.Definition.Description, "schema": json.RawMessage(tool.Definition.Parameters.Bytes()), "available": true})
	}
	writeJSON(w, 200, map[string]any{"tools": items})
}

func (s *Server) handleCreateToolCall(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "tool_operator") {
		return
	}
	key, ok := requireKey(w, r)
	if !ok {
		return
	}
	var input struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	body, _ := json.Marshal(input)
	if resource, duplicate, err := s.keys.Lookup(principal(r).ID, key, r.Method, r.URL.Path, body); err != nil {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	} else if duplicate {
		call, err := s.calls.Load(resource)
		if err != nil {
			writeJSON(w, 202, map[string]any{"call_id": resource, "status": "accepted"})
		} else {
			writeJSON(w, 202, map[string]any{"call_id": resource, "status": call.Status, "revision": call.Revision})
		}
		return
	}
	args, err := root.NewJSONObject(input.Arguments)
	if err != nil {
		writeError(w, requestID(r), 422, "invalid_arguments", "arguments must be a JSON object")
		return
	}
	tools, err := s.availableTools(r)
	if err != nil {
		writeError(w, requestID(r), 503, "engine_unavailable", "tool catalog is unavailable")
		return
	}
	var selected *domain.AvailableTool
	for i := range tools {
		if string(tools[i].Definition.Name) == input.Tool && tools[i].Identity.Kind != domain.ToolKindHost {
			selected = &tools[i]
			break
		}
	}
	if selected == nil {
		writeError(w, requestID(r), 404, "unknown_tool", "tool is not registered")
		return
	}
	if s.deps.Validation == nil || s.deps.Validation.Validate(selected.Definition, args) != nil {
		writeError(w, requestID(r), 422, "invalid_arguments", "arguments do not match the tool schema")
		return
	}
	auto := s.deps.Approval != nil && s.deps.Approval.AutoApproves(selected.Identity)
	if auto && s.deps.Tools == nil {
		writeError(w, requestID(r), 503, "tool_unavailable", "tool executor is unavailable")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, requestID(r), 500, "internal_error", "unable to create call")
		return
	}
	resource, duplicate, err := s.keys.Claim(principal(r).ID, key, r.Method, r.URL.Path, body, id)
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	}
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save call intent")
		return
	}
	if duplicate {
		call, err := s.calls.Load(resource)
		if err != nil {
			writeJSON(w, 202, map[string]any{"call_id": resource, "status": "accepted"})
		} else {
			writeJSON(w, 202, map[string]any{"call_id": resource, "status": call.Status, "revision": call.Revision})
		}
		return
	}
	call := toolCall{ID: resource, Owner: principal(r).ID, Tool: input.Tool, Identity: selected.Identity, Args: append(json.RawMessage(nil), args.Bytes()...), Status: "pending_approval", Revision: 1}
	if auto {
		call.Decision = "approve"
	}
	if err := s.calls.Save(call); err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save call intent")
		return
	}
	if auto {
		if !s.startBackground(func() { s.runDirectCall(resource) }) {
			writeError(w, requestID(r), 503, "server_stopping", "server is stopping")
			return
		}
	}
	writeJSON(w, 202, map[string]any{"call_id": resource, "status": call.Status, "revision": call.Revision})
}

func (s *Server) getCall(w http.ResponseWriter, r *http.Request) (toolCall, bool) {
	id := r.PathValue("id")
	if _, err := domain.NewSessionID(id); err != nil {
		writeError(w, requestID(r), 400, "invalid_id", "invalid call ID")
		return toolCall{}, false
	}
	call, err := s.calls.Load(id)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, requestID(r), 404, "not_found", "call not found")
		return toolCall{}, false
	}
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to load call")
		return toolCall{}, false
	}
	p := principal(r)
	if call.Owner != p.ID && !p.Can("approver") {
		writeError(w, requestID(r), 403, "forbidden", "call belongs to another principal")
		return toolCall{}, false
	}
	return call, true
}

func (s *Server) handleGetToolCall(w http.ResponseWriter, r *http.Request) {
	if !principal(r).Can("tool_operator") && !principal(r).Can("approver") {
		writeError(w, requestID(r), 403, "forbidden", "insufficient role")
		return
	}
	call, ok := s.getCall(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, call)
}

func (s *Server) handleToolDecision(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, requestID(r), 400, "invalid_id", "invalid call ID")
		return
	}
	lock := s.callLock(id)
	lock.Lock()
	defer lock.Unlock()
	call, ok := s.getCall(w, r)
	if !ok {
		return
	}
	body, _ := json.Marshal(input)
	if resource, duplicate, err := s.keys.Lookup(principal(r).ID, key, r.Method, r.URL.Path, body); err != nil {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	} else if duplicate {
		writeJSON(w, 202, map[string]any{"call_id": resource, "status": call.Status, "revision": call.Revision})
		return
	}
	if call.Revision != input.ExpectedRevision {
		writeError(w, requestID(r), 409, "stale_revision", "call revision changed")
		return
	}
	if call.Status != "pending_approval" || call.Decision != "" {
		writeError(w, requestID(r), 409, "invalid_state", "call is not awaiting a decision")
		return
	}
	if input.Decision == "approve" && s.deps.Tools == nil {
		writeError(w, requestID(r), 503, "tool_unavailable", "tool executor is unavailable")
		return
	}
	resource, duplicate, err := s.keys.Claim(principal(r).ID, key, r.Method, r.URL.Path, body, id)
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, requestID(r), 409, "idempotency_conflict", "key already used for another request")
		return
	}
	if err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save decision")
		return
	}
	if duplicate {
		writeJSON(w, 202, map[string]any{"call_id": resource, "status": call.Status, "revision": call.Revision})
		return
	}
	call.Decision = input.Decision
	if input.Decision == "deny" {
		call.Status = "rejected"
		call.Result = &domain.ToolOutcome{Content: "tool call denied", IsError: true}
	}
	call.Revision++
	if err := s.calls.Save(call); err != nil {
		writeError(w, requestID(r), 500, "storage_error", "unable to save decision")
		return
	}
	if input.Decision == "approve" {
		if !s.startBackground(func() { s.runDirectCall(id) }) {
			writeError(w, requestID(r), 503, "server_stopping", "server is stopping")
			return
		}
	}
	writeJSON(w, 202, map[string]any{"call_id": id, "status": call.Status, "revision": call.Revision})
}
