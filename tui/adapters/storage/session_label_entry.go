package storage

type sessionLabelEntry struct {
	Title        string `json:"title,omitempty"`
	TitleDerived bool   `json:"title_derived,omitempty"`
	Archived     bool   `json:"archived,omitempty"`
	About        string `json:"about,omitempty"`
}
