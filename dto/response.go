package dto

import "time"

type Response struct {
	ProtocolVersion int       `json:"protocol_version"`
	RequestID       string    `json:"request_id"`
	Tool            string    `json:"tool"`
	Status          string    `json:"status"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	DurationMS      int64     `json:"duration_ms"`
	Output          any       `json:"output"`
	Error           *Failure  `json:"error"`
}
