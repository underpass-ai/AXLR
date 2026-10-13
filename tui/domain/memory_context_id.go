package domain

import "errors"

// MemoryContextID names the context the memory engine opened for one
// session; the console passes it on every memory call the session makes
// (decision D3 of the lifecycle integration design).
type MemoryContextID string

// NewMemoryContextID validates an id the engine returned.
func NewMemoryContextID(value string) (MemoryContextID, error) {
	id := MemoryContextID(value)
	return id, id.Validate()
}

// Validate refuses an empty id, one longer than 256 bytes and one with
// spaces or control characters.
func (id MemoryContextID) Validate() error {
	if !engineToken(string(id)) {
		return errors.New("memory context id must be 1 to 256 bytes without spaces or control characters")
	}
	return nil
}
