package dto

type SearchArgs struct {
	Pattern      string `json:"pattern"`
	Path         string `json:"path"`
	Glob         string `json:"glob"`
	Literal      bool   `json:"literal"`
	IgnoreCase   bool   `json:"ignore_case"`
	ContextLines int    `json:"context_lines"`
	MaxResults   int    `json:"max_results"`
	Offset       int    `json:"offset"`
	MaxBytes     int    `json:"max_bytes"`
}
