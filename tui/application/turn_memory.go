package application

// TurnMemory is a memory the turn recorded in KMP.
type TurnMemory struct {
	About string `json:"about"`
	Key   string `json:"idempotency_key"`
}
