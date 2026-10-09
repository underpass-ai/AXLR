package dto

type ListEntryOutput struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size *int64 `json:"size,omitempty"`
}

type ListOutput struct {
	Entries      []ListEntryOutput `json:"entries"`
	NextOffset   int               `json:"next_offset,omitempty"`
	Truncated    bool              `json:"truncated"`
	LimitReached bool              `json:"limit_reached,omitempty"`
}
