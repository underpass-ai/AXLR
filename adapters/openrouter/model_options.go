package openrouter

import (
	"bytes"
	"encoding/json"
	"errors"
)

// ModelOptions are request fields OpenRouter reads for one model, sent as
// configured: Provider is its provider routing object (order, sort,
// max_price...), Reasoning its reasoning object, and MaxTokens caps the
// completion, zero leaving it to the provider. Without options a request
// carries none of them and OpenRouter picks the endpoint: on 8 October 2026
// it sent z-ai/glm-5.3-flash to OpenInference at $3.109 per million output
// tokens and 19 tokens per second, when InferenceNet served it at $0.50
// and 120 to 180. Embedded in requestDTO, they encode as its last fields.
type ModelOptions struct {
	Provider  json.RawMessage `json:"provider,omitempty"`
	Reasoning json.RawMessage `json:"reasoning,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

// copyModelOptions keeps the client's own copy, refusing what OpenRouter
// could not read: Provider and Reasoning must be JSON objects.
func copyModelOptions(models map[string]ModelOptions) (map[string]ModelOptions, error) {
	copied := make(map[string]ModelOptions, len(models))
	for model, options := range models {
		for _, raw := range []json.RawMessage{options.Provider, options.Reasoning} {
			var object map[string]json.RawMessage
			if len(raw) > 0 && (json.Unmarshal(raw, &object) != nil || object == nil) {
				return nil, errors.New("OpenRouter provider and reasoning options must be JSON objects")
			}
		}
		if options.MaxTokens < 0 {
			return nil, errors.New("OpenRouter max_tokens option must not be negative")
		}
		copied[model] = ModelOptions{Provider: bytes.Clone(options.Provider), Reasoning: bytes.Clone(options.Reasoning), MaxTokens: options.MaxTokens}
	}
	return copied, nil
}
