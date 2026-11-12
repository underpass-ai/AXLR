package diagnostics

import (
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// toolCallErrorBytes bounds a failed call's text in its app log entry, so
// the whole entry (the names before it, about 200 bytes) stays within
// appLogLineBytes instead of being cut with a marker.
const toolCallErrorBytes = 1536

// ToolCallLog writes one app log entry per finished tool call: INFO when
// it succeeded, WARN with what it answered when it failed. Until 12
// November 2026 the log had no line for a tool call at all, and a bot's
// ten refused memory writes left no trace an operator could read.
type ToolCallLog struct {
	Log *AppLog
}

// ToolCallFinished logs the call as
//
//	tool call: session S, call C, tool T (plugin/tool), decision D, ok in 12ms
//	tool call: session S, call C, tool T, decision D, failed in 3ms: <error>
//
// The record has no field for arguments, prompt or model text; a failed
// call's answer may still quote an argument, and is redacted like every
// entry.
func (l ToolCallLog) ToolCallFinished(record application.ToolCallRecord) {
	if l.Log == nil {
		return
	}
	var line strings.Builder
	line.WriteString("tool call: session " + string(record.Session) + ", call " + string(record.Call) + ", tool " + string(record.Tool))
	if record.PluginTool != "" {
		line.WriteString(" (" + record.PluginTool + ")")
	}
	decision := string(record.Decision)
	if decision == "" {
		decision = "none"
	}
	line.WriteString(", decision " + decision)
	level, status := AppInfo, "ok"
	if record.Failed {
		level, status = AppWarn, "failed"
	}
	line.WriteString(", " + status + " in " + record.Duration.Round(time.Millisecond).String())
	if record.Failed && record.Error != "" {
		line.WriteString(": " + application.ToolCallErrorText(record.Error, toolCallErrorBytes))
	}
	l.Log.write(level, line.String())
}
