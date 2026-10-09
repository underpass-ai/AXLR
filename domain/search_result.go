package domain

// SearchMatch is one matching line. Column is the 1-based byte column of the
// first match; Truncated says the text or a context line was cut.
type SearchMatch struct {
	Path      string
	Line      int
	Column    int
	Text      string
	Before    []string
	After     []string
	Truncated bool
}

// SearchResult holds the matches after Offset; More says another match
// follows them. The counts cover the part of the walk the page needed.
type SearchResult struct {
	Matches      []SearchMatch
	Offset       int
	More         bool
	FilesScanned int
	FilesSkipped int
	LimitReached bool
}
