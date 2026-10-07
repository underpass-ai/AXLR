package openrouter

type requestDTO struct {
	Model    string       `json:"model"`
	Messages []messageDTO `json:"messages"`
	Tools    []toolDTO    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
	// StreamOptions is sent only to endpoints other than OpenRouter.
	StreamOptions *streamOptionsDTO `json:"stream_options,omitempty"`
}

type streamOptionsDTO struct {
	IncludeUsage bool `json:"include_usage"`
}
