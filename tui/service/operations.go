package service

import (
	"encoding/json"
	"net/http"
)

// operation is the shared, closed inventory for HTTP, gRPC and MCP. Local and
// plugin calls create the same durable approval intent as CreateToolCall.
type operation struct {
	Name, Tool, Description, Method, Path string
	Fields                                map[string]string
	Required                              []string
}

func operations() []operation {
	makeOp := func(name, tool, description, method, path string, required []string, fields map[string]string) operation {
		fields["idempotency_key"] = "string"
		fields["request_id"] = "string"
		return operation{name, "axlr_" + tool, description, method, path, fields, required}
	}
	return []operation{
		makeOp("Read", "read", "Request an approved local file read.", "POST", "/v1/tool-calls", []string{"arguments", "idempotency_key"}, map[string]string{"arguments": "object"}),
		makeOp("Write", "write", "Request an approved local file write.", "POST", "/v1/tool-calls", []string{"arguments", "idempotency_key"}, map[string]string{"arguments": "object"}),
		makeOp("Edit", "edit", "Request an approved local file edit.", "POST", "/v1/tool-calls", []string{"arguments", "idempotency_key"}, map[string]string{"arguments": "object"}),
		makeOp("Exec", "exec", "Request an approved local program execution.", "POST", "/v1/tool-calls", []string{"arguments", "idempotency_key"}, map[string]string{"arguments": "object"}),
		makeOp("ListPlugins", "list_plugins", "List exact registered external tool identities and schemas.", "GET", "/v1/tools", nil, map[string]string{}),
		makeOp("CallPlugin", "call_plugin", "Request one exact registered plugin/tool pair under approval policy.", "POST", "/v1/tool-calls", []string{"plugin_id", "tool_name", "arguments", "idempotency_key"}, map[string]string{"plugin_id": "string", "tool_name": "string", "arguments": "object"}),
		makeOp("CreateSession", "create_session", "Create a normal service session.", "POST", "/v1/sessions", []string{"model"}, map[string]string{"model": "string"}),
		makeOp("GetSession", "get_session", "Read an owned session, transcript and pending calls.", "GET", "/v1/sessions/{session_id}", []string{"session_id"}, map[string]string{"session_id": "string"}),
		makeOp("StartTurn", "start_turn", "Start a model turn with revision and idempotency guards.", "POST", "/v1/sessions/{session_id}/turns", []string{"session_id", "prompt", "expected_revision", "idempotency_key"}, map[string]string{"session_id": "string", "prompt": "string", "expected_revision": "integer"}),
		makeOp("StreamEvents", "stream_events", "Replay and briefly follow ordered session events; return a resume cursor. MCP progress notifications carry live events when a progress token is supplied.", "GET", "/v1/sessions/{session_id}/events", []string{"session_id"}, map[string]string{"session_id": "string", "after": "integer", "max_events": "integer", "wait_ms": "integer"}),
		makeOp("DecideSessionTool", "decide_session_tool", "Approve or deny one pending session call.", "POST", "/v1/sessions/{session_id}/approvals/{call_id}", []string{"session_id", "call_id", "decision", "expected_revision", "idempotency_key"}, map[string]string{"session_id": "string", "call_id": "string", "decision": "string", "expected_revision": "integer"}),
		makeOp("CancelSessionTurn", "cancel_session_turn", "Cancel a session turn without rolling back completed effects.", "POST", "/v1/sessions/{session_id}/cancel", []string{"session_id", "expected_revision"}, map[string]string{"session_id": "string", "expected_revision": "integer"}),
		makeOp("ListTools", "list_tools", "List local and registered MCP tools, optionally filtered by origin.", "GET", "/v1/tools", nil, map[string]string{"origin": "string"}),
		makeOp("CreateToolCall", "create_tool_call", "Create a durable call intent subject to approval policy.", "POST", "/v1/tool-calls", []string{"tool", "arguments", "idempotency_key"}, map[string]string{"tool": "string", "arguments": "object"}),
		makeOp("GetToolCall", "get_tool_call", "Read a direct call's status and result.", "GET", "/v1/tool-calls/{call_id}", []string{"call_id"}, map[string]string{"call_id": "string"}),
		makeOp("DecideToolCall", "decide_tool_call", "Approve or deny one durable direct call.", "POST", "/v1/tool-calls/{call_id}/decisions", []string{"call_id", "decision", "expected_revision", "idempotency_key"}, map[string]string{"call_id": "string", "decision": "string", "expected_revision": "integer"}),
		makeOp("Liveness", "liveness", "Read service liveness after client authentication.", "GET", "/livez", nil, map[string]string{}),
		makeOp("Readiness", "readiness", "Check model configuration and required engine discovery.", "GET", "/readyz", nil, map[string]string{}),
	}
}

func (op operation) schema() map[string]any {
	properties := map[string]any{}
	for name, kind := range op.Fields {
		field := map[string]any{"type": kind}
		if kind == "integer" {
			field["minimum"] = 0
			field["maximum"] = ^uint64(0)
		}
		if name == "wait_ms" {
			field["maximum"] = 30000
		}
		if name == "max_events" {
			field["minimum"], field["maximum"] = 1, 256
		}
		if name == "idempotency_key" {
			field["pattern"] = keyPattern.String()
		}
		properties[name] = field
	}
	required := op.Required
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

// operationResult retains exact JSON numbers across protobuf and MCP. Errors
// carry the existing service error envelope rather than a second vocabulary.
type operationResult struct {
	StatusCode int             `json:"status_code"`
	RequestID  string          `json:"request_id"`
	Body       json.RawMessage `json:"body"`
}

func resultError(code int, request, kind, message string) operationResult {
	body, _ := json.Marshal(map[string]any{"error": map[string]string{"code": kind, "message": message, "request_id": request}})
	return operationResult{code, request, body}
}

func findOperation(name string) (operation, bool) {
	for _, op := range operations() {
		if op.Name == name {
			return op, true
		}
	}
	return operation{}, false
}

type responseBuffer struct {
	header http.Header
	status int
	body   []byte
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *responseBuffer) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.body = append(w.body, data...)
	return len(data), nil
}
