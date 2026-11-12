package domain

import (
	"encoding/json"
	"sort"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// toolFailureValueRunes is the shortest argument value taken out of a
// refusal before refusals are compared: a shorter one (a kind, a number)
// would erase words of the message itself.
const toolFailureValueRunes = 3

// ToolFailureStreak reads how the running turn's last answers ended: from
// the newest back, the answers whose every call failed alike, one count
// per answer, so the count is how many times the model sent the failure,
// having read it every time but the first. An answer with a call that
// worked, another failure, a call over the turn's budget or no call ends
// it, and so does the person's word; a budget or restore note does not.
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
		switch message.Role {
		case axlr.RoleUser:
			if JoinsRunningTurn(message.Content) {
				continue
			}
			return streak
		case axlr.RoleAssistant:
			alike, failure, ok := answerFailure(message, calls)
			if !ok || (streak.Count > 0 && alike != key) {
				return streak
			}
			if streak.Count == 0 {
				streak, key = ToolFailureStreak{Tool: message.ToolCalls[0].Name, Failure: failure}, alike
			}
			streak.Count++
		}
	}
	return streak
}

// answerFailure is the failure every call of an answer ended in, with the
// key that tells it from another; ok is false when the answer has no call
// or one that did not fail that way.
func answerFailure(answer axlr.Message, calls map[axlr.ToolCallID]PendingTool) (alike, failure string, ok bool) {
	if len(answer.ToolCalls) == 0 {
		return "", "", false
	}
	for i, call := range answer.ToolCalls {
		record, known := calls[call.ID]
		if !known || record.Outcome == nil || !record.Outcome.IsError || strings.HasPrefix(string(record.Outcome.Content), OverBudgetOutcomePrefix) {
			return "", "", false
		}
		text, key := callFailure(record)
		if i > 0 && key != alike {
			return "", "", false
		}
		alike, failure = key, text
	}
	return alike, failure, true
}

// callFailure is a failed call's error as the model read it, and the key
// two failures share when they are the same. A runtime result (the call
// ran: a command that exited non-zero) is the same failure only for the
// same arguments, its why aside, and the same envelope without its request
// id and times: different commands that both exit 1 silently are not a
// loop. A refusal (the console's {"call_id":…,"error":…}, a plugin's
// text) is the same failure when its text is, once the call's id and the
// values of its own arguments are taken out: on 12 November 2026 a bot
// sent axlr_remember again and again with another invented about.
func callFailure(record PendingTool) (failure, key string) {
	content := record.Outcome.Content
	tool := string(record.Call.Name) + "\x00"
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &fields) != nil {
		failure = strings.TrimSpace(string(content))
		return failure, tool + withoutCallValues(failure, record.Call)
	}
	if _, envelope := fields["protocol_version"]; envelope {
		for _, field := range []string{"request_id", "started_at", "finished_at", "duration_ms"} {
			delete(fields, field)
		}
		data, _ := json.Marshal(fields)
		return string(data), tool + string(data) + "\x00" + argumentsWithoutWhy(record.Call)
	}
	if json.Unmarshal(fields["error"], &failure) != nil || failure == "" {
		failure = strings.TrimSpace(string(content))
	}
	return failure, tool + withoutCallValues(failure, record.Call)
}

// argumentsWithoutWhy is the call's arguments as compact JSON, keys in
// order, without the why a model words anew for the same call.
func argumentsWithoutWhy(call axlr.ToolCall) string {
	var arguments map[string]any
	if json.Unmarshal(call.Arguments.Bytes(), &arguments) != nil {
		return string(call.Arguments.Bytes())
	}
	delete(arguments, "why")
	data, _ := json.Marshal(arguments)
	return string(data)
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
