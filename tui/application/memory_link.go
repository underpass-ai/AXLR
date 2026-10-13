package application

// MemoryLink is a relation the model proposes from a new memory to a ref the
// recall exposed; the console refuses refs the recall did not show.
type MemoryLink struct {
	Ref, Rel, Why, Evidence string
}
