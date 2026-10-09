package domain

// SearchCommand searches the text files under Scope line by line. Offset
// matches are skipped, so a page continues where the previous one stopped
// while the files are unchanged. MaxFiles and MaxScanBytes bound the walk.
type SearchCommand struct {
	Scope        ScopePath
	Pattern      SearchPattern
	Glob         NameGlob
	ContextLines int
	MaxResults   int
	Offset       int
	MaxBytes     ByteLimit
	MaxFiles     int
	MaxScanBytes int64
}
