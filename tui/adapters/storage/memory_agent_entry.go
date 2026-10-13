package storage

// memoryAgentEntry is one store's registered agent.
type memoryAgentEntry struct {
	AgentID         string `json:"agent_id"`
	RegistrationKey string `json:"registration_key"`
	RegisteredAt    string `json:"registered_at"`
}
