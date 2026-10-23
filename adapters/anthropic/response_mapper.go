package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// mapResponse reads a whole reply, or the one a stream assembled: its text
// blocks joined, its tool_use blocks as calls, and its thinking and
// redacted_thinking blocks kept unmodified as the message's reasoning.
func mapResponse(wire responseDTO, rawBlocks []json.RawMessage, host string) (domain.CompletionResult, error) {
	if wire.Role != "" && wire.Role != string(domain.RoleAssistant) {
		return domain.CompletionResult{}, errors.New("Anthropic response is not an assistant message")
	}
	message := domain.Message{Role: domain.RoleAssistant}
	var texts []string
	var reasoning []json.RawMessage
	seen := make(map[domain.ToolCallID]bool)
	cut := wire.StopReason == "max_tokens" || wire.StopReason == "model_context_window_exceeded"
	for i, block := range wire.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			id, err := domain.NewToolCallID(block.ID)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			if seen[id] {
				return domain.CompletionResult{}, errors.New("duplicate Anthropic tool call ID")
			}
			seen[id] = true
			name, err := domain.NewToolName(block.Name)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			arguments, err := provider.ToolArguments(string(block.Input), cut)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			message.ToolCalls = append(message.ToolCalls, domain.ToolCall{ID: id, Name: name, Arguments: arguments})
		case "thinking", "redacted_thinking":
			if i < len(rawBlocks) && len(rawBlocks[i]) > 0 {
				reasoning = append(reasoning, rawBlocks[i])
				continue
			}
			raw, err := json.Marshal(block)
			if err != nil {
				return domain.CompletionResult{}, err
			}
			reasoning = append(reasoning, raw)
		}
	}
	content := strings.Join(texts, "")
	if wire.StopReason == "refusal" {
		// The classifier declined: an empty reply would fail as a message
		// and tell nobody why.
		note := "[AXLR] The model declined this request"
		if wire.StopDetails != nil && wire.StopDetails.Category != "" {
			note += " (" + wire.StopDetails.Category + ")"
		}
		if wire.StopDetails != nil && wire.StopDetails.Explanation != "" {
			note += ": " + provider.SanitizeProviderMessage(wire.StopDetails.Explanation)
		}
		content = join(content, note+".")
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
			return domain.CompletionResult{}, errors.New("malformed Anthropic thinking blocks")
		}
		message.Reasoning = value
	}
	if err := message.Validate(); err != nil {
		return domain.CompletionResult{}, fmt.Errorf("Anthropic response: %w", err)
	}
	result := domain.CompletionResult{Message: message, FinishReason: finishReason(wire.StopReason), Provider: host}
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

// mapUsage adds the cache reads and writes to the fresh input tokens, so
// PromptTokens is the whole prompt as everywhere else in AXLR. Anthropic
// reports no cost and no separate count of thinking tokens.
func mapUsage(wire usageDTO) *domain.TokenUsage {
	prompt := wire.InputTokens + wire.CacheReadInputTokens + wire.CacheCreationInputTokens
	return &domain.TokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: wire.OutputTokens,
		TotalTokens:      prompt + wire.OutputTokens,
		CachedTokens:     wire.CacheReadInputTokens,
		CacheWriteTokens: wire.CacheCreationInputTokens,
	}
}
