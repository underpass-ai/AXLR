package domain

// Entry types a listing reports. A symlink is reported, never followed.
const (
	EntryFile    = "file"
	EntryDir     = "dir"
	EntrySymlink = "symlink"
	EntryOther   = "other"
)

// ListEntry is one workspace entry; Size is meaningful for files only.
type ListEntry struct {
	Path string
	Type string
	Size int64
}

// ListResult holds the entries after Offset; More says another follows.
type ListResult struct {
	Entries      []ListEntry
	Offset       int
	More         bool
	LimitReached bool
}
