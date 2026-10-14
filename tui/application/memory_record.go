package application

// MemoryRecord is one memory with its links, written by the console.
type MemoryRecord struct {
	ID, Kind, Summary, Evidence string
	Links                       []MemoryLink
}
