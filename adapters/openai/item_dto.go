package openai

import "encoding/json"

// itemDTO is one item of the input or the output: a message, a function
// call, a function call output or a reasoning item.
type itemDTO struct {
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"`
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
	// Function calls.
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
	// Reasoning items.
	Summary          []summaryPartDTO `json:"summary,omitempty"`
	EncryptedContent string           `json:"encrypted_content,omitempty"`
}

// outputItemDTO is an output item as the API returns it, with its content
// parts typed and its raw form kept for reasoning items.
type outputItemDTO struct {
	Type             string           `json:"type"`
	ID               string           `json:"id"`
	Status           string           `json:"status"`
	Role             string           `json:"role"`
	Content          []contentPartDTO `json:"content"`
	CallID           string           `json:"call_id"`
	Name             string           `json:"name"`
	Arguments        string           `json:"arguments"`
	Summary          []summaryPartDTO `json:"summary"`
	EncryptedContent string           `json:"encrypted_content"`
	raw              json.RawMessage
}

func (i *outputItemDTO) UnmarshalJSON(data []byte) error {
	type plain outputItemDTO
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = outputItemDTO(decoded)
	i.raw = append(json.RawMessage(nil), data...)
	return nil
}

type contentPartDTO struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type summaryPartDTO struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
