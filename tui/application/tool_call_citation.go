package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Every tool result names the call that produced it, so the model can cite
// it in axlr_request_repair and axlr_request_improvement. Claude models never
// see the IDs of their own calls: on 16 October 2026 Haiku 5.5 cited
// "axlr_forge_tool#1 (hello_probe, first attempt)" and the console refused
// it. A runtime envelope names the call in request_id; a host result in a
// leading call_id. Both are fixed when the result is recorded, so the
// prefix a provider cached never changes.

// UnnamedRequestID is the request_id of a runtime call that no model call
// asked for, and of every result recorded before calls were named.
const UnnamedRequestID = "tui-tool"

// maxCitableCalls bounds the calls a refused citation lists.
const maxCitableCalls = 8

type toolCallKey struct{}

// WithToolCall carries the model's call ID to the tool runtime.
func WithToolCall(ctx context.Context, id root.ToolCallID) context.Context {
	return context.WithValue(ctx, toolCallKey{}, id)
}

// ToolCallOf is the model's call the runtime is executing, if any.
func ToolCallOf(ctx context.Context) (root.ToolCallID, bool) {
	id, ok := ctx.Value(toolCallKey{}).(root.ToolCallID)
	return id, ok && id != ""
}

// citeCall names the call in a JSON object result that does not name it
// yet. Text results, such as a denial, are left as they are: a refused
// citation lists the calls the model can cite instead.
func citeCall(outcome domain.ToolOutcome, id root.ToolCallID) domain.ToolOutcome {
	content := string(outcome.Content)
	var object map[string]json.RawMessage
	if id == "" || !strings.HasPrefix(content, "{") || json.Unmarshal([]byte(content), &object) != nil {
		return outcome
	}
	if _, named := object["request_id"]; named {
		return outcome
	}
	if _, named := object["call_id"]; named {
		return outcome
	}
	encoded, _ := json.Marshal(string(id))
	rest := strings.TrimLeft(content[1:], " \t\r\n")
	if strings.HasPrefix(rest, "}") {
		outcome.Content = root.Text(`{"call_id":` + string(encoded) + `}`)
	} else {
		outcome.Content = root.Text(`{"call_id":` + string(encoded) + `,` + rest)
	}
	return outcome
}

// namedCall is the key and call ID a recorded result names, if it names
// one; a result recorded before calls were named names none.
func namedCall(original any) (string, string, bool) {
	object, ok := original.(map[string]any)
	if !ok {
		return "", "", false
	}
	if id, ok := object["call_id"].(string); ok && id != "" {
		return "call_id", id, true
	}
	if id, ok := object["request_id"].(string); ok && id != "" && id != UnnamedRequestID {
		return "request_id", id, true
	}
	return "", "", false
}

// citableCalls lists the session's most recent calls the model may cite,
// newest first, as "ID (tool, outcome)".
func citableCalls(state domain.SessionState, citable func(domain.PendingTool) bool) string {
	var listed []string
	for i := len(state.Activity) - 1; i >= 0 && len(listed) < maxCitableCalls; i-- {
		record := state.Activity[i]
		if record.Outcome == nil || record.Decision == domain.DecisionDeny || !citable(record) {
			continue
		}
		outcome := "ok"
		if record.Outcome.IsError {
			outcome = "error"
		}
		listed = append(listed, fmt.Sprintf("%s (%s, %s)", record.Call.ID, record.Call.Name, outcome))
	}
	return strings.Join(listed, ", ")
}

// uncitedRefusal refuses a citation that is not a call of this session and
// lists the calls that are.
func uncitedRefusal(state domain.SessionState, id string, citable func(domain.PendingTool) bool) string {
	refusal := fmt.Sprintf("call %s is not in this session; cite IDs of tool calls you made here: each result names its call in call_id, or in request_id for local_* tools", id)
	if calls := citableCalls(state, citable); calls != "" {
		refusal += ". Recent calls: " + calls
	}
	return refusal
}

// ownCall reports whether the call is of AXLR's own tools, not of an MCP
// plugin.
func ownCall(state domain.SessionState) func(domain.PendingTool) bool {
	return func(record domain.PendingTool) bool {
		tool, known, err := hostFindTool(state.ToolSnapshot, record.Call.Name)
		return err == nil && known && tool.Identity.Kind != domain.ToolKindPlugin
	}
}
