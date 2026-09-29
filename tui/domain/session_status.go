package domain

type SessionStatus string

const (
	StatusIdle        SessionStatus = "idle"
	StatusStreaming   SessionStatus = "streaming"
	StatusApproval    SessionStatus = "approval"
	StatusInterrupted SessionStatus = "interrupted"
	StatusComplete    SessionStatus = "complete"
)
