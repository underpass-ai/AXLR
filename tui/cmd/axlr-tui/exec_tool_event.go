package main

import (
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// maxExecToolArguments bounds the arguments a tool line carries: past it
// they are left out whole rather than cut into invalid JSON. A failure's
// text is kept whole on one line, up to application.MaxToolCallErrorBytes:
// until 12 November 2026 it was cut to its first line and 240 characters,
// and a refused memory write reached the bot's panel as "…is a teammat…",
// without the part that said what to do.
const maxExecToolArguments = 8 << 10

// execToolEvent is a call's line: requested with the arguments the model
// gave, so a reader that follows the run sees what the call asks for, or
// completed with how it ended. A call that reaches a plugin's tool, by its
// own name, its opaque alias or through axlr_call_tool, names that tool as
// plugin/tool, resolved against the session's snapshot as the run resolves it.
func execToolEvent(tool domain.PendingTool, snapshot []domain.AvailableTool) execEvent {
	event := execEvent{Type: "tool.requested", Tool: string(tool.Call.Name), Call: string(tool.Call.ID), Decision: string(tool.Decision), PluginTool: application.CallPluginTool(snapshot, tool.Call)}
	if tool.Outcome == nil {
		if arguments := tool.Call.Arguments.Compact().Bytes(); len(arguments) <= maxExecToolArguments {
			event.Arguments = arguments
		}
		return event
	}
	event.Type = "tool.completed"
	event.Status = "ok"
	if tool.Outcome.IsError {
		event.Status = "failed"
		event.Error = application.ToolCallErrorText(string(tool.Outcome.Content), application.MaxToolCallErrorBytes)
	}
	return event
}
