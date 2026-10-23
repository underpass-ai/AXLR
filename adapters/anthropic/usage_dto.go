package anthropic

// usageDTO is the usage of a reply: input_tokens counts only the prompt
// tokens read fresh, cache_read_input_tokens those read from the prompt
// cache and cache_creation_input_tokens those written to it; the three add
// up to the prompt. In a stream, message_start carries the input counts and
// the final message_delta the output count, repeating the input counts.
type usageDTO struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
