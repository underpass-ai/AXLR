package dto

type FileChange struct {
	Path        string `json:"path"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
	Created     bool   `json:"created,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}
