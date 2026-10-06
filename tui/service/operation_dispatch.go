package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type operationSnapshotKey struct{}

// decodeOperation validates the transport envelope without converting integers
// to float64. Actual tool arguments and revisions use the existing handlers.
func decodeOperation(op operation, data []byte) (map[string]json.RawMessage, bool) {
	if len(data) > 4<<20 {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return nil, false
	}
	for _, required := range op.Required {
		if _, ok := fields[required]; !ok {
			return nil, false
		}
	}
	for name, value := range fields {
		kind, ok := op.Fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, false
		}
		switch kind {
		case "string":
			var text string
			if json.Unmarshal(value, &text) != nil {
				return nil, false
			}
		case "integer":
			var number uint64
			if json.Unmarshal(value, &number) != nil {
				return nil, false
			}
		case "object":
			if _, err := root.NewJSONObject(value); err != nil {
				return nil, false
			}
		}
	}
	return fields, true
}

func stringField(fields map[string]json.RawMessage, name string) string {
	var value string
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func (s *Server) invokeOperation(ctx context.Context, op operation, data []byte, state *tls.ConnectionState, emit func(Event) error) operationResult {
	request, _ := newID()
	p, authenticated := authenticate(&http.Request{TLS: state}, s.principals)
	if !authenticated {
		return resultError(403, request, "forbidden", "certificate is not authorized")
	}
	if len(data) > 4<<20 {
		return resultError(413, request, "body_too_large", "request body exceeds 4 MiB")
	}
	fields, ok := decodeOperation(op, data)
	if !ok {
		return resultError(400, request, "invalid_json", "arguments do not match the operation schema")
	}
	if id := stringField(fields, "request_id"); safeRequestID(id) {
		request = id
	}
	if _, exists := fields["idempotency_key"]; exists && !keyPattern.MatchString(stringField(fields, "idempotency_key")) {
		return resultError(400, request, "invalid_idempotency_key", "Idempotency-Key must be 16–128 safe ASCII characters")
	}
	if op.Name == "StreamEvents" {
		return s.streamOperation(ctx, fields, p, request, emit)
	}
	body := map[string]json.RawMessage{}
	for key, value := range fields {
		switch key {
		case "request_id", "idempotency_key", "session_id", "call_id", "origin":
		default:
			body[key] = value
		}
	}
	path := op.Path
	for _, name := range []string{"session_id", "call_id"} {
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(stringField(fields, name)))
	}
	if op.Name == "ListTools" || op.Name == "ListPlugins" {
		origin := stringField(fields, "origin")
		if op.Name == "ListPlugins" {
			origin = "mcp"
		}
		path += "?" + url.Values{"origin": []string{origin}}.Encode()
	}
	switch op.Name {
	case "Read", "Write", "Edit", "Exec":
		name, _ := json.Marshal("local_" + strings.ToLower(op.Name))
		body["tool"] = name
	case "CallPlugin":
		if !p.Can("tool_operator") {
			return resultError(403, request, "forbidden", "insufficient role")
		}
		// Reuse a persisted identity on replay even when discovery is down.
		// The ordinary handler still checks the complete request digest.
		prior, exists, err := s.keys.record(p.ID, stringField(fields, "idempotency_key"))
		if err != nil {
			return resultError(500, request, "storage_error", "unable to read call intent")
		}
		var exact string
		if exists {
			call, err := s.calls.Load(prior.Resource)
			if err != nil || prior.Path != op.Path || call.Identity.Kind != domain.ToolKindPlugin || call.Identity.Plugin.PluginID.String() != stringField(fields, "plugin_id") || call.Identity.Plugin.ToolName.String() != stringField(fields, "tool_name") {
				return resultError(409, request, "idempotency_conflict", "key already used for another request")
			}
			exact = call.Tool
		} else if s.deps.Catalog == nil {
			return resultError(503, request, "engine_unavailable", "tool catalog is unavailable")
		} else {
			tools, err := s.deps.Catalog.Snapshot(ctx)
			if err != nil {
				return resultError(503, request, "engine_unavailable", "tool catalog is unavailable")
			}
			ctx = context.WithValue(ctx, operationSnapshotKey{}, tools)
			for _, tool := range tools {
				if tool.Identity.Kind == domain.ToolKindPlugin && tool.Identity.Plugin.PluginID.String() == stringField(fields, "plugin_id") && tool.Identity.Plugin.ToolName.String() == stringField(fields, "tool_name") {
					exact = string(tool.Definition.Name)
					break
				}
			}
		}
		if exact == "" {
			return resultError(404, request, "unknown_tool", "tool is not registered")
		}
		delete(body, "plugin_id")
		delete(body, "tool_name")
		body["tool"], _ = json.Marshal(exact)
	}
	encoded, _ := json.Marshal(body)
	r, err := http.NewRequestWithContext(ctx, op.Method, path, bytes.NewReader(encoded))
	if err != nil {
		return resultError(400, request, "invalid_id", "invalid resource ID")
	}
	r.TLS = state
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-Id", request)
	r.Header.Set("Idempotency-Key", stringField(fields, "idempotency_key"))
	w := &responseBuffer{header: http.Header{}}
	if op.Name == "Liveness" || op.Name == "Readiness" {
		s.ProbeHandler().ServeHTTP(w, r)
	} else {
		s.authenticated(s.apiRoutes()).ServeHTTP(w, r)
	}
	if len(w.body) == 0 {
		w.body = []byte("null")
	}
	return operationResult{w.status, request, json.RawMessage(bytes.TrimSpace(w.body))}
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	op, ok := findOperation(r.PathValue("verb"))
	if !ok {
		writeError(w, requestID(r), 404, "unknown_operation", "operation is not registered")
		return
	}
	var data json.RawMessage
	if !readJSON(w, r, &data) {
		return
	}
	// The HTTP operation endpoint has the same JSON envelope as the gRPC and
	// MCP adapters. The original REST routes retain their existing payloads.
	result := s.invokeOperation(r.Context(), op, data, r.TLS, nil)
	code := result.StatusCode
	if code == http.StatusNoContent {
		code = http.StatusOK // retain the envelope for probe operations
	}
	writeJSON(w, code, result)
}
