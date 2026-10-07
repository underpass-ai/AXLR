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
// An absent or empty request is the empty object, so verbs without required
// fields accept a bare MCP call, an empty BytesValue or a bodiless POST.
func decodeOperation(op operation, data []byte) (map[string]json.RawMessage, bool) {
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}
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

// invokeOperation runs one verb for any transport. request is the caller's
// default request ID; a safe request_id argument replaces it.
func (s *Server) invokeOperation(ctx context.Context, op operation, data []byte, state *tls.ConnectionState, request string, emit func(Event) error) operationResult {
	if !safeRequestID(request) {
		request, _ = newID()
	}
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
	// Resource IDs are checked before they become path segments, so an empty
	// ID is an invalid_id error rather than an unmatched route. A session's
	// pending call ID is model-assigned; the handler matches it exactly.
	for _, name := range []string{"session_id", "call_id"} {
		if _, exists := fields[name]; !exists {
			continue
		}
		value := stringField(fields, name)
		_, err := domain.NewSessionID(value)
		if name == "call_id" && op.Name == "DecideSessionTool" && value != "" {
			err = nil
		}
		if err != nil {
			return resultError(400, request, "invalid_id", "invalid "+strings.TrimSuffix(name, "_id")+" ID")
		}
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
	api, probe := s.operationHandlers()
	w := &responseBuffer{header: http.Header{}}
	if op.Name == "Liveness" || op.Name == "Readiness" {
		probe.ServeHTTP(w, r)
	} else {
		api.ServeHTTP(w, r)
	}
	return operationOutcome(op, w, request)
}

// operationOutcome keeps the service error vocabulary on every transport: a
// probe failure or any response without the error envelope still names a code.
func operationOutcome(op operation, w *responseBuffer, request string) operationResult {
	status := w.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	body := bytes.TrimSpace(w.body)
	if len(body) == 0 {
		body = []byte("null")
	}
	if !json.Valid(body) {
		if status < 400 {
			status = http.StatusInternalServerError
		}
		return resultError(status, request, "unexpected_response", strings.ToLower(http.StatusText(status)))
	}
	if status < 400 {
		return operationResult{status, request, body}
	}
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil && envelope.Error.Code != "" {
		return operationResult{status, request, body}
	}
	if op.Name == "Liveness" || op.Name == "Readiness" {
		return resultError(status, request, "not_ready", "model configuration or engine discovery is not ready")
	}
	return resultError(status, request, "unexpected_response", strings.ToLower(http.StatusText(status)))
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	op, ok := findOperation(r.PathValue("verb"))
	if !ok {
		writeError(w, requestID(r), 404, "unknown_operation", "operation is not registered")
		return
	}
	data := json.RawMessage("{}")
	if r.ContentLength != 0 && !readJSON(w, r, &data) {
		return
	}
	// The HTTP operation endpoint has the same JSON envelope as the gRPC and
	// MCP adapters. The original REST routes retain their existing payloads.
	// The request header is the default ID; a body request_id replaces it and
	// the response header follows the envelope.
	result := s.invokeOperation(r.Context(), op, data, r.TLS, requestID(r), nil)
	w.Header().Set("X-Request-Id", result.RequestID)
	code := result.StatusCode
	if code == http.StatusNoContent {
		code = http.StatusOK // retain the envelope for probe operations
	}
	writeJSON(w, code, result)
}
