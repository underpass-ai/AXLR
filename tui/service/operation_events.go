package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func (s *Server) streamOperation(ctx context.Context, fields map[string]json.RawMessage, p Principal, request string, emit func(Event) error) operationResult {
	if !p.Can("session_client") {
		return resultError(403, request, "forbidden", "insufficient role")
	}
	id := stringField(fields, "session_id")
	if _, err := domain.NewSessionID(id); err != nil {
		return resultError(400, request, "invalid_id", "invalid session ID")
	}
	session, err := s.loadSession(ctx, id)
	if isNotFound(err) {
		return resultError(404, request, "not_found", "session not found")
	}
	if err != nil {
		return resultError(500, request, "storage_error", "unable to load session")
	}
	if session.Export().Owner != p.ID && !p.Can("admin") {
		return resultError(403, request, "forbidden", "session belongs to another principal")
	}
	select {
	case s.streamSlots <- struct{}{}:
		defer func() { <-s.streamSlots }()
	default:
		return resultError(429, request, "too_many_streams", "too many event streams")
	}
	var after, limit, wait uint64
	_ = json.Unmarshal(fields["after"], &after)
	_ = json.Unmarshal(fields["max_events"], &limit)
	_ = json.Unmarshal(fields["wait_ms"], &wait)
	if _, exists := fields["max_events"]; exists && limit == 0 {
		return resultError(400, request, "invalid_stream_limits", "max_events must be 1–256")
	}
	if limit == 0 {
		limit = 128
	}
	if limit > 256 || wait > 30000 {
		return resultError(400, request, "invalid_stream_limits", "max_events must be 1–256 and wait_ms at most 30000")
	}
	timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
	defer timer.Stop()
	collected := []Event{}
	finish := func(reason string) operationResult {
		body, _ := json.Marshal(map[string]any{"events": collected, "next_sequence": after, "end_reason": reason})
		return operationResult{200, request, body}
	}
	for {
		events, changed, err := s.events.Read(id, after)
		if err != nil {
			return resultError(400, request, "invalid_cursor", "event cursor is invalid")
		}
		for _, event := range events {
			if emit != nil {
				if err := emit(event); err != nil {
					return resultError(503, request, "stream_unavailable", "event delivery interrupted")
				}
			}
			collected = append(collected, event)
			after = event.Sequence
			if uint64(len(collected)) == limit {
				return finish("event_limit")
			}
		}
		if wait == 0 {
			return finish("snapshot")
		}
		select {
		case <-ctx.Done():
			return finish("cancelled")
		case <-s.root.Done():
			return finish("server_stopping")
		case <-timer.C:
			return finish("wait_elapsed")
		case <-changed:
		}
	}
}
