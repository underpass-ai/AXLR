package application

// MemoryRelation is one stored or proposed relation between two refs.
type MemoryRelation struct {
	From, To, Rel, Why string
	// Dimension and Scope name the coordinate a proposal rests on.
	Dimension, Scope string
}
