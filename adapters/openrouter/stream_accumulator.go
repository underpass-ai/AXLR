package openrouter

import (
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

// streamAccumulator holds fragments until the provider finishes the assistant message.
type streamAccumulator struct {
	content string
	calls   map[int]toolCallDTO
	finish  string
	usage   *usageDTO
	total   int
}

func (a *streamAccumulator) Add(data []byte) ([]domain.Text, error) {
	a.total += len(data)
	if a.total > maxResponseBytes {
		return nil, errors.New("OpenRouter stream exceeds 8 MiB")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("malformed OpenRouter stream UTF-8")
	}
	var chunk struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role      string  `json:"role"`
				Content   *string `json:"content"`
				ToolCalls []struct {
					Index    *int   `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *usageDTO `json:"usage"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, errors.New("malformed OpenRouter stream chunk")
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		var provider struct {
			Code int `json:"code"`
		}
		_ = json.Unmarshal(chunk.Error, &provider)
		if provider.Code == 0 {
			provider.Code = 502
		}
		return nil, classifyProviderError(provider.Code)
	}
	if chunk.Choices == nil {
		return nil, errors.New("OpenRouter stream chunk has no choices")
	}
	var deltas []domain.Text
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			return nil, errors.New("unsupported OpenRouter stream choice index")
		}
		if a.finish != "" {
			return nil, errors.New("OpenRouter stream has a delta after finish")
		}
		if choice.Delta.Role != "" && choice.Delta.Role != "assistant" {
			return nil, errors.New("OpenRouter stream is not an assistant message")
		}
		if choice.Delta.Content != nil {
			text, err := domain.NewText(*choice.Delta.Content)
			if err != nil {
				return nil, err
			}
			a.content += string(text)
			if text != "" {
				deltas = append(deltas, text)
			}
		}
		for _, fragment := range choice.Delta.ToolCalls {
			if fragment.Index == nil || *fragment.Index < 0 {
				return nil, errors.New("invalid OpenRouter tool call index")
			}
			if a.calls == nil {
				a.calls = make(map[int]toolCallDTO)
			}
			call := a.calls[*fragment.Index]
			call.ID += fragment.ID
			if fragment.Type != "" {
				if fragment.Type != "function" {
					return nil, errors.New("unsupported OpenRouter tool call type")
				}
				call.Type = fragment.Type
			}
			call.Function.Name += fragment.Function.Name
			call.Function.Arguments += fragment.Function.Arguments
			a.calls[*fragment.Index] = call
		}
		if choice.FinishReason != nil {
			a.finish = *choice.FinishReason
		}
	}
	if chunk.Usage != nil {
		a.usage = chunk.Usage
	}
	return deltas, nil
}

func (a *streamAccumulator) Result() (domain.CompletionResult, error) {
	if a.finish == "" {
		return domain.CompletionResult{}, errors.New("OpenRouter stream ended without a finish reason")
	}
	message := messageDTO{Role: "assistant", Content: &a.content}
	indices := make([]int, 0, len(a.calls))
	for index := range a.calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		message.ToolCalls = append(message.ToolCalls, a.calls[index])
	}
	return mapResponse(responseDTO{Choices: []choiceDTO{{Message: message, FinishReason: a.finish}}, Usage: a.usage})
}
