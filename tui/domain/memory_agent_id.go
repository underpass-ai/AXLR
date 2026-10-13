package domain

import "errors"

// MemoryAgentID names the agent the console registered in a memory store.
type MemoryAgentID string

// NewMemoryAgentID validates an id the engine returned.
func NewMemoryAgentID(value string) (MemoryAgentID, error) {
	id := MemoryAgentID(value)
	return id, id.Validate()
}

// Validate refuses an empty id, one longer than 256 bytes and one with
// spaces or control characters.
func (id MemoryAgentID) Validate() error {
	if !engineToken(string(id)) {
		return errors.New("memory agent id must be 1 to 256 bytes without spaces or control characters")
	}
	return nil
}
