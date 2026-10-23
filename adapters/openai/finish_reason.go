package openai

import "github.com/underpass-ai/AXLR/domain"

// finishReason reads a response's status in the vocabulary the rest of AXLR
// knows: stop, tool_calls and length.
func finishReason(wire responseDTO, calls int) domain.FinishReason {
	switch wire.Status {
	case "incomplete":
		if wire.IncompleteDetails != nil {
			switch wire.IncompleteDetails.Reason {
			case "max_output_tokens":
				return "length"
			case "":
			default:
				return domain.FinishReason(wire.IncompleteDetails.Reason)
			}
		}
		return "length"
	case "", "completed":
		if calls > 0 {
			return "tool_calls"
		}
		return "stop"
	}
	return domain.FinishReason(wire.Status)
}
