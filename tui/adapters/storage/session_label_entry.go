package storage

type sessionLabelEntry struct {
	Title    string `json:"title,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	About    string `json:"about,omitempty"`
}
