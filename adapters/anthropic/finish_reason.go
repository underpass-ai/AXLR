package anthropic

import "github.com/underpass-ai/AXLR/domain"

// finishReason reads a stop reason in the vocabulary the rest of AXLR knows
// from OpenAI-shaped replies: stop, tool_calls and length. A refusal keeps
// its name; anything else passes through.
func finishReason(stop string) domain.FinishReason {
	switch stop {
	case "end_turn", "stop_sequence", "pause_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens", "model_context_window_exceeded":
		return "length"
	}
	return domain.FinishReason(stop)
}
