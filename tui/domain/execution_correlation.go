package domain

import "errors"

// ExecutionCorrelation is how the control plane names one execution
// (docs/runtime-contract.md, "Correlation"): the bot, the task, the
// execution and the trace it belongs to. AXLR carries the ids unchanged
// into every event, trace record and session record of the execution, and
// never into the model's prompt. Each id is safe ASCII of bounded length,
// so it can be a label anywhere; any of them may be empty.
type ExecutionCorrelation struct {
	BotID       string
	TaskID      string
	ExecutionID string
	TraceID     string
}

// MaxCorrelationIDBytes bounds each id.
const MaxCorrelationIDBytes = 128

func (c ExecutionCorrelation) Validate() error {
	for _, id := range []string{c.BotID, c.TaskID, c.ExecutionID, c.TraceID} {
		if !SafeCorrelationID(id) {
			return errors.New("a correlation id is letters, digits and -_.:/@+=, at most 128 characters")
		}
	}
	return nil
}

// IsZero reports a correlation with no id.
func (c ExecutionCorrelation) IsZero() bool {
	return c.BotID == "" && c.TaskID == "" && c.ExecutionID == "" && c.TraceID == ""
}

// SafeCorrelationID accepts an empty id, or one of letters, digits and
// -_.:/@+= within MaxCorrelationIDBytes: a UUID, a W3C trace id, a slug.
func SafeCorrelationID(id string) bool {
	if len(id) > MaxCorrelationIDBytes {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == ':' || c == '/' || c == '@' || c == '+' || c == '=':
		default:
			return false
		}
	}
	return true
}
