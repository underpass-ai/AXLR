package openrouter

type requestDTO struct {
	Model    string       `json:"model"`
	Messages []messageDTO `json:"messages"`
	Tools    []toolDTO    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}
