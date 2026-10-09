package dto

type ListArgs struct {
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	Recursive  bool   `json:"recursive"`
	MaxDepth   int    `json:"max_depth"`
	MaxEntries int    `json:"max_entries"`
	Offset     int    `json:"offset"`
	MaxBytes   int    `json:"max_bytes"`
}
