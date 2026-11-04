package main

import (
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// Bounds of what a tool line carries: arguments past their bound are left
// out whole rather than cut into invalid JSON, and a failure's text is cut
// to its first line.
const (
	maxExecToolArguments = 8 << 10
	maxExecToolError     = 240
)

// execToolEvent is a call's line: requested with the arguments the model
// gave, so a reader that follows the run sees what the call asks for, or
// completed with how it ended.
func execToolEvent(tool domain.PendingTool) execEvent {
	event := execEvent{Type: "tool.requested", Tool: string(tool.Call.Name), Call: string(tool.Call.ID), Decision: string(tool.Decision)}
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
		event.Error = execToolError(string(tool.Outcome.Content))
	}
	return event
}

// execToolError is a failure's first non-empty line, cut short.
func execToolError(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > maxExecToolError {
			line = string([]rune(line)[:maxExecToolError-1]) + "…"
		}
		return line
	}
	return ""
}
