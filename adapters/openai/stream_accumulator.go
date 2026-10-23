package openai

import (
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// streamAccumulator holds the events of a Responses stream until its terminal
// event: response.completed, response.incomplete or response.failed, which
// carries the whole response. The text deltas are shown as they arrive; the
// output items the terminal event repeats are the record, and the items of
// response.output_item.done stand in when it carries none.
type streamAccumulator struct {
	items    map[int]outputItemDTO
	final    *responseDTO
	total    int
	provider string
}

// Add reads one event and returns the text deltas to show.
func (a *streamAccumulator) Add(data []byte) ([]domain.Text, error) {
	a.total += len(data)
	if a.total > provider.MaxResponseBytes {
		return nil, errors.New(a.provider + " stream exceeds 8 MiB")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("malformed " + a.provider + " stream UTF-8")
	}
	var event struct {
		Type        string          `json:"type"`
		Delta       string          `json:"delta"`
		OutputIndex int             `json:"output_index"`
		Item        *outputItemDTO  `json:"item"`
		Response    *responseDTO    `json:"response"`
		Code        json.RawMessage `json:"code"`
		Message     string          `json:"message"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, errors.New("malformed " + a.provider + " stream event")
	}
	switch event.Type {
	case "error":
		raw, _ := json.Marshal(errorDTO{Code: event.Code, Message: event.Message})
		if err, ok := providerError(raw); ok {
			return nil, err
		}
		return nil, errors.New(a.provider + " stream reported an error")
	case "response.output_text.delta":
		text, err := domain.NewText(event.Delta)
		if err != nil {
			return nil, err
		}
		if text != "" {
			return []domain.Text{text}, nil
		}
	case "response.output_item.done":
		if event.Item != nil {
			if a.items == nil {
				a.items = map[int]outputItemDTO{}
			}
			a.items[event.OutputIndex] = *event.Item
		}
	case "response.completed", "response.incomplete", "response.failed":
		if event.Response == nil {
			return nil, errors.New(a.provider + " stream ended without its response")
		}
		a.final = event.Response
	}
	return nil, nil
}

// Done reports whether the terminal event arrived.
func (a *streamAccumulator) Done() bool { return a.final != nil }

// Result assembles the message the stream built.
func (a *streamAccumulator) Result(host string) (domain.CompletionResult, error) {
	if a.final == nil {
		return domain.CompletionResult{}, errors.New(a.provider + " stream ended without a response")
	}
	final := *a.final
	if len(final.Output) == 0 && len(a.items) > 0 {
		indices := make([]int, 0, len(a.items))
		for index := range a.items {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			final.Output = append(final.Output, a.items[index])
		}
	}
	return mapResponse(final, host)
}
