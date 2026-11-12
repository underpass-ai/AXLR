package domain

import (
	"encoding/json"
	"sort"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// toolFailureValueRunes is the shortest argument value taken out of a
// failure before failures are compared: a shorter one (a kind, a number)
// would erase words of the message itself.
const toolFailureValueRunes = 3

// ToolFailureStreak reads how the running turn's last tool calls ended:
// from its newest tool result back, the calls that failed alike, until a
// call that succeeded, another tool's, another failure or the turn's
// start. A budget or restore note does not end it; the person's word does.
func (s Session) ToolFailureStreak() ToolFailureStreak {
	state := s.Export()
	calls := make(map[axlr.ToolCallID]PendingTool, len(state.Activity))
	for _, record := range state.Activity {
		calls[record.Call.ID] = record
	}
	var streak ToolFailureStreak
	key := ""
	for i := len(state.Messages) - 1; i >= 0; i-- {
		message := state.Messages[i]
		if message.Role == axlr.RoleUser {
			if JoinsRunningTurn(message.Content) {
				continue
			}
			break
		}
		if message.Role != axlr.RoleTool {
			continue
		}
		record, known := calls[message.ToolCallID]
		if !known || record.Outcome == nil || !record.Outcome.IsError {
			break
		}
		failure := toolFailureText(record.Outcome.Content)
		alike := string(record.Call.Name) + "\x00" + withoutCallValues(failure, record.Call)
		if streak.Count == 0 {
			streak, key = ToolFailureStreak{Tool: record.Call.Name, Failure: failure}, alike
		} else if alike != key {
			break
		}
		streak.Count++
	}
	return streak
}

// toolFailureText is a failed result's error: the error field of the
// console's refusal ({"call_id":…,"error":…}); a runtime envelope without
// its request id and times, which differ on every run of the same failing
// command; else the text itself.
func toolFailureText(content axlr.Text) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &fields) != nil {
		return strings.TrimSpace(string(content))
	}
	var refusal string
	if json.Unmarshal(fields["error"], &refusal) == nil && refusal != "" {
		return refusal
	}
	if _, envelope := fields["protocol_version"]; envelope {
		for _, key := range []string{"request_id", "started_at", "finished_at", "duration_ms"} {
			delete(fields, key)
		}
		if data, err := json.Marshal(fields); err == nil {
			return string(data)
		}
	}
	return strings.TrimSpace(string(content))
}

// withoutCallValues takes the call's id and the string values of its
// arguments out of its failure, the longest first, so two refusals that
// quote different values of the same mistake read the same.
func withoutCallValues(failure string, call axlr.ToolCall) string {
	values := []string{string(call.ID)}
	var arguments any
	if json.Unmarshal(call.Arguments.Bytes(), &arguments) == nil {
		values = appendStringValues(values, arguments)
	}
	sort.SliceStable(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		if value = strings.TrimSpace(value); len([]rune(value)) >= toolFailureValueRunes {
			failure = strings.ReplaceAll(failure, value, "…")
		}
	}
	return failure
}

// appendStringValues adds every string inside a decoded JSON value.
func appendStringValues(values []string, value any) []string {
	switch typed := value.(type) {
	case string:
		values = append(values, typed)
	case []any:
		for _, item := range typed {
			values = appendStringValues(values, item)
		}
	case map[string]any:
		for _, item := range typed {
			values = appendStringValues(values, item)
		}
	}
	return values
}
