package dto

type SearchMatchOutput struct {
	Path      string   `json:"path"`
	Line      int      `json:"line"`
	Column    int      `json:"column"`
	Text      string   `json:"text"`
	Before    []string `json:"before,omitempty"`
	After     []string `json:"after,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

type SearchOutput struct {
	Matches      []SearchMatchOutput `json:"matches"`
	FilesScanned int                 `json:"files_scanned"`
	FilesSkipped int                 `json:"files_skipped"`
	NextOffset   int                 `json:"next_offset,omitempty"`
	Truncated    bool                `json:"truncated"`
	LimitReached bool                `json:"limit_reached,omitempty"`
}
