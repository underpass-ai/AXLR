package openai

import "encoding/json"

// reasoningConfig asks for readable summaries of the reasoning, so the
// console can show it as it streams; the effort stays the model's default
// unless the client names one.
func reasoningConfig(effort string) json.RawMessage {
	if effort == "" {
		return json.RawMessage(`{"summary":"auto"}`)
	}
	raw, _ := json.Marshal(struct {
		Summary string `json:"summary"`
		Effort  string `json:"effort"`
	}{"auto", effort})
	return raw
}

// encryptedReasoning asks for the reasoning items encrypted, the only form a
// stateless request can send back: without it a replayed item refers to
// state the server did not keep.
var encryptedReasoning = []string{"reasoning.encrypted_content"}
