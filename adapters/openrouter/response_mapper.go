package openrouter

import (
	"errors"

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
		arguments, err := domain.NewJSONObject([]byte(call.Function.Arguments))
		if err != nil {
			return domain.CompletionResult{}, err
		}
		message.ToolCalls = append(message.ToolCalls, domain.ToolCall{ID: id, Name: name, Arguments: arguments})
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
