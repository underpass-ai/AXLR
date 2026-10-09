package terminal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolRowHeadBytes bounds how much of a saved result the transcript inspects.
// Results can be hundreds of kilobytes and rows render on every snapshot.
const toolRowHeadBytes = 1024

const toolRowArgumentWidth = 48

var shortStringField = regexp.MustCompile(`"\w+":"([^"\\]{1,80})"`)

// toolRow condenses one call, its decision and its result into a single line:
// status glyph, tool, a short argument summary, then size, duration and any
// human decision. Automatic approvals are not shown.
func toolRow(s domain.SessionState, call root.ToolCall, record *domain.PendingTool, result *root.Message, theme Theme) transcriptRow {
	label, memory := toolCallPresentation(s, call)
	kind := transcriptRowPlain
	if memory {
		kind = transcriptRowMemory
	}
	glyph, tone := theme.Icon("waiting"), toneWarning
	facts := &toolFacts{Label: label, State: toolRunning}
	var details []string
	switch {
	case record != nil && record.Decision == domain.DecisionDeny:
		glyph, tone = theme.Icon("failed"), toneError
		facts.State = toolDenied
		details = append(details, theme.T(deniedLabel(record)))
	case result != nil:
		glyph, tone = theme.Icon("done"), toneGood
		facts.State = toolDone
		head := string(result.Content[:min(len(result.Content), toolRowHeadBytes)])
		if record != nil && record.Outcome != nil && record.Outcome.IsError || strings.Contains(head, `"status":"failed"`) {
			glyph, tone = theme.Icon("failed"), toneError
			facts.State = toolFailed
		}
		facts.Bytes = len(result.Content)
		details = append(details, formatBytes(len(result.Content)))
		if duration, ok := durationMS(head); ok {
			facts.DurationMS, facts.HasTime = duration, true
			details = append(details, formatDuration(duration))
		}
	case record != nil && record.Decision == "":
		facts.State = toolAwaiting
		details = append(details, theme.T("transcript.toolAwaiting"))
	default:
		details = append(details, theme.T("transcript.toolRunning"))
	}
	if memory && tone == toneGood {
		glyph, tone = theme.Icon("memory"), toneAccent
	}
	if record != nil && record.Decision == domain.DecisionApprove {
		details = append(details, theme.T("transcript.toolApproved"))
	}
	text := label
	args := toolArgumentSummary(call)
	if summary, ok := localListingSummary(call, theme); ok {
		args = summary
	}
	if args != "" {
		text += "  " + args
	}
	text += " · " + strings.Join(details, " · ")
	return transcriptRow{Label: glyph + " ", LabelTone: tone, Text: text, Kind: kind, Indent: true, Tool: facts}
}

// toolArgumentSummary lists the call's top-level scalar values in argument
// order ("README.md", "project:AXLR"); nested objects are left to Info.
func toolArgumentSummary(call root.ToolCall) string {
	raw := call.Arguments.Bytes()
	if call.Name == application.HostCallToolName && len(raw) <= toolRowHeadBytes {
		// The wrapper's name is already the row's label; summarise what it passes on.
		var wrapper struct {
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(raw, &wrapper) == nil {
			raw = wrapper.Arguments
		}
	}
	if len(raw) > toolRowHeadBytes {
		// Large arguments (a file's content) are not decoded; short string
		// fields at their head (a path) still identify the call.
		var values []string
		for _, match := range shortStringField.FindAllStringSubmatch(string(raw[:toolRowHeadBytes]), -1) {
			values = append(values, match[1])
		}
		return ansi.Truncate(singleLine(strings.Join(values, " ")), toolRowArgumentWidth, "…")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return ""
	}
	var values []string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return ""
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return ""
		}
		switch v := value.(type) {
		case string:
			if v != "" {
				values = append(values, v)
			}
		case json.Number, bool:
			values = append(values, fmt.Sprintf("%v=%v", key, v))
		}
	}
	return ansi.Truncate(singleLine(strings.Join(values, " ")), toolRowArgumentWidth, "…")
}

// localListingSummary reads a search as its question, `"pattern" in path
// (glob)`, and a listing as its path, where the scalar summary would run
// pattern, path and glob together.
func localListingSummary(call root.ToolCall, theme Theme) (string, bool) {
	if call.Name != "local_search" && call.Name != "local_list" {
		return "", false
	}
	var args struct {
		Pattern   string `json:"pattern"`
		Path      string `json:"path"`
		Glob      string `json:"glob"`
		Recursive bool   `json:"recursive"`
	}
	if json.Unmarshal(call.Arguments.Bytes(), &args) != nil {
		return "", false
	}
	where := args.Path
	if where == "" {
		where = "."
	}
	summary := where
	if call.Name == "local_search" {
		summary = theme.Tf("transcript.searchIn", args.Pattern, where)
	} else if args.Recursive {
		summary = theme.Tf("transcript.listRecursive", where)
	}
	if args.Glob != "" {
		summary += " (" + args.Glob + ")"
	}
	return ansi.Truncate(singleLine(summary), toolRowArgumentWidth, "…"), true
}

func durationMS(head string) (int64, bool) {
	const key = `"duration_ms":`
	at := strings.Index(head, key)
	if at < 0 {
		return 0, false
	}
	digits := head[at+len(key):]
	end := 0
	for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	value, err := strconv.ParseInt(digits[:end], 10, 64)
	return value, err == nil
}

func formatBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d\u00a0B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f\u00a0KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f\u00a0MB", float64(n)/(1024*1024))
	}
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d\u00a0ms", ms)
	}
	return fmt.Sprintf("%.1f\u00a0s", float64(ms)/1000)
}

// deniedLabel tells a call the console refused, its arguments unreadable or
// its tool unknown, from one the person or the mode denied: on 9 Oct 2026 a
// model's malformed JSON read "denied" as if the person had refused it.
func deniedLabel(record *domain.PendingTool) string {
	if record.Outcome != nil {
		switch content := string(record.Outcome.Content); {
		case strings.HasPrefix(content, "invalid tool invocation rejected: "):
			return "transcript.toolInvalid"
		case strings.HasPrefix(content, "unknown tool "):
			return "transcript.toolUnknown"
		}
	}
	return "transcript.toolDenied"
}
