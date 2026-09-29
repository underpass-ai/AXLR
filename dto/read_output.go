package dto

type ReadOutput struct {
	Content          string `json:"content"`
	StartOffsetBytes int64  `json:"start_offset_bytes"`
	ReturnedBytes    int    `json:"returned_bytes"`
	NextOffsetBytes  int64  `json:"next_offset_bytes"`
	Truncated        bool   `json:"truncated"`
	ContentSHA256    string `json:"content_sha256,omitempty"`
}
