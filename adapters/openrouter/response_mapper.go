package openrouter

import (
	"bytes"
	"errors"

	"github.com/underpass-ai/AXLR/adapters/provider"
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
	if details := bytes.TrimSpace(choice.Message.ReasoningDetails); len(details) > 0 && !bytes.Equal(details, []byte("null")) {
		reasoning, err := domain.NewJSONValue(details)
		if err != nil {
			return domain.CompletionResult{}, errors.New("malformed OpenRouter reasoning details")
		}
		message.Reasoning = reasoning
	}
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
	result := domain.CompletionResult{Message: message, FinishReason: domain.FinishReason(choice.FinishReason), Provider: wire.Provider}
	if wire.Usage != nil {
		result.Usage = mapUsage(*wire.Usage)
	}
	return result, nil
}

func mapUsage(wire usageDTO) *domain.TokenUsage {
	usage := &domain.TokenUsage{
		PromptTokens:     wire.PromptTokens,
		CompletionTokens: wire.CompletionTokens,
		TotalTokens:      wire.TotalTokens,
	}
	if details := wire.PromptTokensDetails; details != nil {
		usage.CachedTokens = details.CachedTokens
		usage.CacheWriteTokens = details.CacheWriteTokens
	}
	if details := wire.CompletionTokensDetails; details != nil {
		usage.ReasoningTokens = details.ReasoningTokens
	}
	if wire.Cost != nil {
		usage.Cost, usage.CostKnown = *wire.Cost, true
		// With the person's own key, cost is OpenRouter's fee and the
		// provider bills the inference to that key: the request cost both.
		// Without it, upstream_inference_cost repeats cost and is not added.
		if wire.IsBYOK && wire.CostDetails != nil {
			usage.Cost += wire.CostDetails.UpstreamInferenceCost
		}
	}
	return usage
}

// outputLimitNote ends an answer the output limit cut short (finish_reason
// "length"), so neither the person nor the model takes it for complete.
const outputLimitNote = provider.OutputLimitNote

// toolArguments reads a call's arguments, repairing what is not a JSON
// object into the shared malformed-arguments object.
func toolArguments(raw string, finish string) (domain.JSONValue, error) {
	return provider.ToolArguments(raw, finish == "length")
}
