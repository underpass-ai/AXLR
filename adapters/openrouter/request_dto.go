package openrouter

type requestDTO struct {
	Model    string       `json:"model"`
	Messages []messageDTO `json:"messages"`
	Tools    []toolDTO    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
	// StreamOptions is sent only to endpoints other than OpenRouter.
	StreamOptions *streamOptionsDTO `json:"stream_options,omitempty"`
	// ChatTemplateKwargs turns a local model's thinking off, as llama.cpp
	// and vLLM read it.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type streamOptionsDTO struct {
	IncludeUsage bool `json:"include_usage"`
}
