package storage

// memoryAgentsRecord is the agent registry file.
type memoryAgentsRecord struct {
	Version int                         `json:"version"`
	Agents  map[string]memoryAgentEntry `json:"agents"`
}
