package application

// ToolCallLogPort hears of every tool call that finished: run, failed,
// denied by the person or refused by the console before it ran. It is the
// operator's readable log; the trace keeps the numbers. A record never
// carries the call's arguments, the prompt or the model's text: only the
// names, the decision, how the call ended and, when it failed, what it
// answered. nil records nothing.
type ToolCallLogPort interface {
	ToolCallFinished(ToolCallRecord)
}
