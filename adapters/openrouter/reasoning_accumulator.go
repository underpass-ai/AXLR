package openrouter

import (
	"encoding/json"
	"errors"
)

// reasoningAccumulator joins the reasoning_details fragments of a stream
// into the blocks OpenRouter would return whole: the fragments of one block
// share its index, their text, summary and data are concatenated in order,
// and every other field (type, id, format, signature) keeps the last value
// sent. On 23 October 2026 claude-haiku-5.5 streamed one reasoning.text
// block as 23 text fragments and a last fragment with only its signature.
type reasoningAccumulator struct {
	blocks []map[string]json.RawMessage
	// positions maps a block's index to its place in blocks.
	positions map[int]int
}

// concatenated are the fields a block streams in pieces.
var concatenated = map[string]bool{"text": true, "summary": true, "data": true}

func (a *reasoningAccumulator) Add(fragments []map[string]json.RawMessage) error {
	for _, fragment := range fragments {
		position := -1
		var index int
		if raw, ok := fragment["index"]; ok && json.Unmarshal(raw, &index) == nil {
			if at, seen := a.positions[index]; seen {
				position = at
			} else {
				if a.positions == nil {
					a.positions = map[int]int{}
				}
				a.positions[index] = len(a.blocks)
			}
		} else if len(a.blocks) > 0 && sameType(a.blocks[len(a.blocks)-1], fragment) {
			position = len(a.blocks) - 1
		}
		if position < 0 {
			a.blocks = append(a.blocks, map[string]json.RawMessage{})
			position = len(a.blocks) - 1
		}
		block := a.blocks[position]
		for name, value := range fragment {
			if string(value) == "null" {
				if _, present := block[name]; !present {
					block[name] = value
				}
				continue
			}
			if concatenated[name] {
				var piece string
				if err := json.Unmarshal(value, &piece); err != nil {
					return errors.New("malformed OpenRouter reasoning fragment")
				}
				var sofar string
				if previous, ok := block[name]; ok && string(previous) != "null" {
					_ = json.Unmarshal(previous, &sofar)
				}
				joined, _ := json.Marshal(sofar + piece)
				block[name] = joined
				continue
			}
			block[name] = value
		}
	}
	return nil
}

func sameType(block, fragment map[string]json.RawMessage) bool {
	return string(block["type"]) == string(fragment["type"])
}

// Result is the reasoning_details array, nil when the stream sent none.
func (a *reasoningAccumulator) Result() json.RawMessage {
	if len(a.blocks) == 0 {
		return nil
	}
	encoded, err := json.Marshal(a.blocks)
	if err != nil {
		return nil
	}
	return encoded
}
