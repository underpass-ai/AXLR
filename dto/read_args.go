package dto

type ReadArgs struct {
	Path        string `json:"path"`
	OffsetBytes int64  `json:"offset_bytes"`
	MaxBytes    int    `json:"max_bytes"`
}
