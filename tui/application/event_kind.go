package application

type EventKind string

const (
	EventSession              EventKind = "session"
	EventProviderActivity     EventKind = "provider_activity"
	EventToolCallProgress     EventKind = "tool_call_progress"
	EventToolExecutionStarted EventKind = "tool_execution_started"
	EventTextDelta            EventKind = "text_delta"
	EventStreamStart          EventKind = "stream_start"
	EventState                EventKind = "state"
	EventToolActivity         EventKind = "tool_activity"
)
