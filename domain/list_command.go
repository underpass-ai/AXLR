package domain

// ListCommand lists the entries under Scope down to Depth levels (1 is the
// directory's own entries). Offset entries are skipped; MaxVisits bounds
// the walk.
type ListCommand struct {
	Scope      ScopePath
	Glob       NameGlob
	Depth      int
	MaxEntries int
	Offset     int
	MaxBytes   ByteLimit
	MaxVisits  int
}
