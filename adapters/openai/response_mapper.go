package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// mapResponse reads a response: its message items' text joined, its
// function calls as tool calls, and its reasoning items, encrypted, kept
// unmodified as the message's reasoning. A reasoning item without encrypted
// content cannot be replayed to a stateless request and is left out.
func mapResponse(wire responseDTO, host string) (domain.CompletionResult, error) {
	if failed, ok := providerError(wire.Error); ok {
		return domain.CompletionResult{}, failed
	}
	message := domain.Message{Role: domain.RoleAssistant}
	var texts, refusals []string
	var reasoning []json.RawMessage
	seen := make(map[domain.ToolCallID]bool)
	cut := wire.Status == "incomplete" && (wire.IncompleteDetails == nil || wire.IncompleteDetails.Reason == "max_output_tokens")
	for _, item := range wire.Output {
		switch item.Type {
		case "message":
			if item.Role != "" && item.Role != string(domain.RoleAssistant) {
				return domain.CompletionResult{}, errors.New("OpenAI response is not an assistant message")
			}
			for _, part := range item.Content {
				switch part.Type {
				case "output_text":
					texts = append(texts, part.Text)
				case "refusal":
					refusals = append(refusals, part.Refusal)
				}
			}
		case "function_call":
			id, err := domain.NewToolCallID(item.CallID)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			if seen[id] {
				return domain.CompletionResult{}, errors.New("duplicate OpenAI tool call ID")
			}
			seen[id] = true
			name, err := domain.NewToolName(item.Name)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			arguments, err := provider.ToolArguments(item.Arguments, cut)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			message.ToolCalls = append(message.ToolCalls, domain.ToolCall{ID: id, Name: name, Arguments: arguments})
		case "reasoning":
			if item.EncryptedContent != "" && len(item.raw) > 0 {
				reasoning = append(reasoning, item.raw)
			}
		}
	}
	content := strings.Join(texts, "")
	for _, refusal := range refusals {
		content = join(content, "[AXLR] The model declined this request: "+provider.SanitizeProviderMessage(refusal))
	}
	if cut && len(message.ToolCalls) == 0 {
		content = join(content, provider.OutputLimitNote)
	}
	text, err := domain.NewText(content)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	message.Content = text
	if len(reasoning) > 0 {
		encoded, err := json.Marshal(reasoning)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		value, err := domain.NewJSONValue(encoded)
		if err != nil {
			return domain.CompletionResult{}, errors.New("malformed OpenAI reasoning items")
		}
		message.Reasoning = value
	}
	if err := message.Validate(); err != nil {
		return domain.CompletionResult{}, fmt.Errorf("OpenAI response: %w", err)
	}
	result := domain.CompletionResult{Message: message, FinishReason: finishReason(wire, len(message.ToolCalls)), Provider: host}
	if wire.Usage != nil {
		result.Usage = mapUsage(*wire.Usage)
	}
	return result, nil
}

func join(content, note string) string {
	if content != "" {
		content += "\n\n"
	}
	return content + note
}

// mapUsage reads the counts; OpenAI reports no cost, so it stays unknown.
func mapUsage(wire usageDTO) *domain.TokenUsage {
	usage := &domain.TokenUsage{PromptTokens: wire.InputTokens, CompletionTokens: wire.OutputTokens, TotalTokens: wire.TotalTokens}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = wire.InputTokens + wire.OutputTokens
	}
	if wire.InputTokensDetails != nil {
		usage.CachedTokens = wire.InputTokensDetails.CachedTokens
	}
	if wire.OutputTokensDetails != nil {
		usage.ReasoningTokens = wire.OutputTokensDetails.ReasoningTokens
	}
	return usage
}
