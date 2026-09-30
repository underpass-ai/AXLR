package application

type EventKind string

const (
	EventTextDelta    EventKind = "text_delta"
	EventStreamStart  EventKind = "stream_start"
	EventState        EventKind = "state"
	EventToolActivity EventKind = "tool_activity"
)
