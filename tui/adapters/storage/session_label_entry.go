package storage

type sessionLabelEntry struct {
	Title        string `json:"title,omitempty"`
	TitleDerived bool   `json:"title_derived,omitempty"`
	Archived     bool   `json:"archived,omitempty"`
	About        string `json:"about,omitempty"`
	// ImportedFrom and ImportedAfter: see domain.SessionLabel.
	ImportedFrom  string `json:"imported_from,omitempty"`
	ImportedAfter int    `json:"imported_after,omitempty"`
}
