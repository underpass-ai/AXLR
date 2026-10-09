package openrouter

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

func mapResponse(wire responseDTO) (domain.CompletionResult, error) {
	if len(wire.Choices) == 0 {
		return domain.CompletionResult{}, errors.New("OpenRouter response has no choices")
	}
	choice := wire.Choices[0]
	if choice.Message.Role != string(domain.RoleAssistant) {
		return domain.CompletionResult{}, errors.New("OpenRouter response is not an assistant message")
	}
	message := domain.Message{Role: domain.RoleAssistant}
	if choice.Message.Content != nil {
		content, err := domain.NewText(*choice.Message.Content)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		message.Content = content
	}
	seen := make(map[domain.ToolCallID]bool, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "function" {
			return domain.CompletionResult{}, errors.New("unsupported OpenRouter tool call type")
		}
		id, err := domain.NewToolCallID(call.ID)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		if seen[id] {
			return domain.CompletionResult{}, errors.New("duplicate OpenRouter tool call ID")
		}
		seen[id] = true
		name, err := domain.NewToolName(call.Function.Name)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		arguments, err := toolArguments(call.Function.Arguments, choice.FinishReason)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		message.ToolCalls = append(message.ToolCalls, domain.ToolCall{ID: id, Name: name, Arguments: arguments})
	}
	// A call cut by the limit says so in its own arguments; whole calls
	// stand as they are.
	if choice.FinishReason == "length" && len(message.ToolCalls) == 0 {
		if message.Content != "" {
			message.Content += "\n\n"
		}
		message.Content += outputLimitNote
	}
	if err := message.Validate(); err != nil {
		return domain.CompletionResult{}, err
	}
	result := domain.CompletionResult{Message: message, FinishReason: domain.FinishReason(choice.FinishReason)}
	if wire.Usage != nil {
		result.Usage = &domain.TokenUsage{
			PromptTokens:     wire.Usage.PromptTokens,
			CompletionTokens: wire.Usage.CompletionTokens,
			TotalTokens:      wire.Usage.TotalTokens,
		}
		if details := wire.Usage.PromptTokensDetails; details != nil {
			result.Usage.CachedTokens = details.CachedTokens
			result.Usage.CacheWriteTokens = details.CacheWriteTokens
		}
	}
	return result, nil
}

// outputLimitNote ends an answer the output limit cut short (finish_reason
// "length"), so neither the person nor the model takes it for complete.
const outputLimitNote = "[AXLR] The model's output limit cut this answer short."

// malformedArgumentsKey names the object that stands in for a call's
// arguments when they are not a JSON object. The console's tool resolution
// (tui/application/host_tool_resolution.go) refuses a call that carries it
// with the error inside, so the model is told and can send the call again;
// keep the two names in step.
const malformedArgumentsKey = "axlr_malformed_arguments"

// malformedArgumentsPrefixBytes bounds the part of unreadable arguments kept
// to show the model which call it was.
const malformedArgumentsPrefixBytes = 512

type malformedArgumentsDTO struct {
	Error            string `json:"error"`
	RawPrefix        string `json:"raw_prefix"`
	CutByOutputLimit bool   `json:"cut_by_output_limit,omitempty"`
}

// toolArguments reads a call's arguments. Empty arguments are the empty
// object of a call without parameters. Arguments that are not a JSON object,
// cut short by the output limit or mistyped by the model, would fail the
// whole operation; they are kept instead as a valid object that names the
// error, for the turn to answer the call with a tool error.
func toolArguments(raw string, finish string) (domain.JSONValue, error) {
	if strings.TrimSpace(raw) == "" {
		return domain.NewJSONObject([]byte("{}"))
	}
	if arguments, err := domain.NewJSONObject([]byte(raw)); err == nil {
		return arguments, nil
	}
	malformed := malformedArgumentsDTO{Error: "arguments must be a JSON object", RawPrefix: utf8Prefix(raw, malformedArgumentsPrefixBytes), CutByOutputLimit: finish == "length"}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		malformed.Error = err.Error()
	}
	encoded, err := json.Marshal(map[string]malformedArgumentsDTO{malformedArgumentsKey: malformed})
	if err != nil {
		return domain.JSONValue{}, err
	}
	return domain.NewJSONObject(encoded)
}

// utf8Prefix keeps at most maximum bytes of text on a rune boundary.
func utf8Prefix(text string, maximum int) string {
	if len(text) <= maximum {
		return text
	}
	for maximum > 0 && !utf8.RuneStart(text[maximum]) {
		maximum--
	}
	return text[:maximum]
}
