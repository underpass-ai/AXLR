package anthropic

import "encoding/json"

type requestDTO struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    []blockDTO   `json:"system,omitempty"`
	Messages  []messageDTO `json:"messages"`
	Tools     []toolDTO    `json:"tools,omitempty"`
	Stream    bool         `json:"stream,omitempty"`
	// Thinking is the adaptive configuration, nil when thinking is off.
	Thinking json.RawMessage `json:"thinking,omitempty"`
	// CacheControl asks for the automatic breakpoint, which the API places
	// on the last block and moves forward as the conversation grows.
	CacheControl *cacheControlDTO `json:"cache_control,omitempty"`
}

type cacheControlDTO struct {
	Type string `json:"type"`
}
