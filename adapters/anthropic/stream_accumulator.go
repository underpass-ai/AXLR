package anthropic

import (
	"encoding/json"
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// streamBlock is one content block as a stream builds it: the fields of its
// content_block_start, the text, thinking, signature and partial JSON its
// deltas append.
type streamBlock struct {
	start       blockDTO
	text        string
	thinking    string
	signature   string
	partialJSON string
	hasJSON     bool
}

// streamAccumulator holds the events of a Messages stream until message_stop.
type streamAccumulator struct {
	blocks   map[int]*streamBlock
	usage    usageDTO
	hasUsage bool
	stop     string
	details  *stopDetailsDTO
	stopped  bool
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
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			Role  string    `json:"role"`
			Usage *usageDTO `json:"usage"`
		} `json:"message"`
		ContentBlock *blockDTO `json:"content_block"`
		Delta        struct {
			Type        string          `json:"type"`
			Text        string          `json:"text"`
			PartialJSON string          `json:"partial_json"`
			Thinking    string          `json:"thinking"`
			Signature   string          `json:"signature"`
			StopReason  string          `json:"stop_reason"`
			StopDetails *stopDetailsDTO `json:"stop_details"`
		} `json:"delta"`
		Usage *usageDTO       `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, errors.New("malformed " + a.provider + " stream event")
	}
	switch event.Type {
	case "error":
		if err, ok := providerError(event.Error); ok {
			return nil, err
		}
		return nil, errors.New(a.provider + " stream reported an error")
	case "message_start":
		if event.Message.Role != "" && event.Message.Role != "assistant" {
			return nil, errors.New(a.provider + " stream is not an assistant message")
		}
		if event.Message.Usage != nil {
			a.mergeUsage(*event.Message.Usage)
		}
	case "content_block_start":
		if event.ContentBlock == nil {
			return nil, errors.New(a.provider + " stream block has no content")
		}
		if a.blocks == nil {
			a.blocks = map[int]*streamBlock{}
		}
		block := &streamBlock{start: *event.ContentBlock, text: event.ContentBlock.Text}
		if event.ContentBlock.Thinking != nil {
			block.thinking = *event.ContentBlock.Thinking
		}
		a.blocks[event.Index] = block
	case "content_block_delta":
		block := a.blocks[event.Index]
		if block == nil {
			return nil, errors.New(a.provider + " stream delta without its block")
		}
		switch event.Delta.Type {
		case "text_delta":
			text, err := domain.NewText(event.Delta.Text)
			if err != nil {
				return nil, err
			}
			block.text += string(text)
			if text != "" {
				return []domain.Text{text}, nil
			}
		case "input_json_delta":
			block.partialJSON += event.Delta.PartialJSON
			block.hasJSON = true
		case "thinking_delta":
			block.thinking += event.Delta.Thinking
		case "signature_delta":
			block.signature += event.Delta.Signature
		}
	case "message_delta":
		if event.Delta.StopReason != "" {
			a.stop = event.Delta.StopReason
		}
		if event.Delta.StopDetails != nil {
			a.details = event.Delta.StopDetails
		}
		if event.Usage != nil {
			a.mergeUsage(*event.Usage)
		}
	case "message_stop":
		a.stopped = true
	}
	return nil, nil
}

// mergeUsage keeps the latest value of each count the stream reported: the
// output count grows with every message_delta, the input counts arrive in
// message_start and may be repeated.
func (a *streamAccumulator) mergeUsage(wire usageDTO) {
	a.hasUsage = true
	if wire.InputTokens > 0 {
		a.usage.InputTokens = wire.InputTokens
	}
	if wire.CacheReadInputTokens > 0 {
		a.usage.CacheReadInputTokens = wire.CacheReadInputTokens
	}
	if wire.CacheCreationInputTokens > 0 {
		a.usage.CacheCreationInputTokens = wire.CacheCreationInputTokens
	}
	if wire.OutputTokens > 0 {
		a.usage.OutputTokens = wire.OutputTokens
	}
}

// Stopped reports whether message_stop arrived.
func (a *streamAccumulator) Stopped() bool { return a.stopped }

// Result assembles the message the stream built.
func (a *streamAccumulator) Result(host string) (domain.CompletionResult, error) {
	if a.stop == "" {
		return domain.CompletionResult{}, errors.New(a.provider + " stream ended without a stop reason")
	}
	indices := make([]int, 0, len(a.blocks))
	for index := range a.blocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	wire := responseDTO{Role: "assistant", StopReason: a.stop, StopDetails: a.details}
	var raw []json.RawMessage
	for _, index := range indices {
		block := a.blocks[index]
		dto := block.start
		dto.Text = block.text
		dto.CacheControl = nil
		var input json.RawMessage
		switch dto.Type {
		case "tool_use":
			// The joined fragments are the input; a call cut by the output
			// limit leaves them unreadable, and the mapper repairs them.
			input = dto.Input
			if block.hasJSON {
				input = json.RawMessage(block.partialJSON)
			} else if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			dto.Input = nil
		case "thinking":
			thinking := block.thinking
			dto.Thinking = &thinking
			if block.signature != "" {
				dto.Signature = block.signature
			}
		}
		encoded, err := json.Marshal(dto)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		dto.Input = input
		wire.Content = append(wire.Content, dto)
		raw = append(raw, encoded)
	}
	if a.hasUsage {
		usage := a.usage
		wire.Usage = &usage
	}
	return mapResponse(wire, raw, host)
}
