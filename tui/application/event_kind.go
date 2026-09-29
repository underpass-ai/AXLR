package application

type EventKind string

const (
	EventTextDelta    EventKind = "text_delta"
	EventState        EventKind = "state"
	EventToolActivity EventKind = "tool_activity"
)
