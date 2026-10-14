package terminal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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

var shortStringField = regexp.MustCompile(`"(\w+)":"([^"\\]{1,80})"`)

var pathField = regexp.MustCompile(`"path":\s*"([^"\\]{1,240})"`)

// summaryHidden names arguments a row never lists: the model's reason (the
// approval card and the turn record carry it) and file bodies, which would
// push the path out of the row.
var summaryHidden = map[string]bool{"why": true}

// fileBodies are the local file tools' body arguments, hidden from their
// rows; another tool's "content" may be what identifies the call.
var fileBodies = map[string]bool{"content": true, "old_text": true, "new_text": true}

func hiddenFromRow(tool root.ToolName, key string) bool {
	return summaryHidden[key] || (fileBodies[key] && (tool == "local_write" || tool == "local_edit"))
}

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
		label := deniedLabel(record)
		if label == "transcript.toolOverBudget" {
			// Nobody denied it; the editorial summary counts it as not run.
			facts.State = toolFailed
		}
		details = append(details, theme.T(label))
	case result != nil:
		glyph, tone = theme.Icon("done"), toneGood
		facts.State = toolDone
		head := string(result.Content[:min(len(result.Content), toolRowHeadBytes)])
		if record != nil && record.Outcome != nil && record.Outcome.IsError || strings.Contains(head, `"status":"failed"`) {
			glyph, tone = theme.Icon("failed"), toneError
			facts.State = toolFailed
		}
		facts.Bytes = resultBytes(call, result.Content)
		details = append(details, formatBytes(facts.Bytes))
		if duration, ok := durationMS(head); ok {
			facts.DurationMS, facts.HasTime = duration, true
			details = append(details, formatDuration(duration))
		}
	case record != nil && record.Decision == "":
		facts.State = toolAwaiting
		// Only the first undecided call is the card's; the calls behind it
		// wait their turn and may need no approval at all.
		details = append(details, theme.T(awaitingLabel(s, call)))
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
	} else if summary, ok := localExecSummary(call); ok {
		args = summary
	} else if summary, ok := localFileSummary(call); ok {
		args = summary
	} else if summary, ok := stepDoneSummary(call); ok {
		args = summary
	}
	if args != "" {
		text += "  " + args
	}
	text += " · " + strings.Join(details, " · ")
	return transcriptRow{Label: glyph + " ", LabelTone: tone, Text: text, Kind: kind, Indent: true, Tool: facts}
}

// awaitingLabel says "waiting for approval" for the call the approval card
// is about, the first undecided one, and "queued" for the undecided calls
// behind it. On 12 October 2026 an axlr_skill call that needs no approval
// read "waiting for approval" while a local_exec card was open.
func awaitingLabel(s domain.SessionState, call root.ToolCall) string {
	for _, p := range s.Activity {
		if p.Outcome == nil && p.Decision == "" {
			if p.Call.ID == call.ID {
				return "transcript.toolAwaiting"
			}
			return "transcript.toolQueued"
		}
	}
	return "transcript.toolAwaiting"
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
		path := false
		for _, match := range shortStringField.FindAllStringSubmatch(string(raw[:toolRowHeadBytes]), -1) {
			if hiddenFromRow(call.Name, match[1]) {
				continue
			}
			path = path || match[1] == "path"
			values = summaryValue(values, match[1], match[2])
		}
		if !path {
			// A model may send the body first: the path is looked up after it.
			if match := pathField.FindSubmatch(raw); match != nil {
				values = summaryValue(values, "path", string(match[1]))
			}
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
		if name, _ := key.(string); hiddenFromRow(call.Name, name) {
			continue
		}
		switch v := value.(type) {
		case string:
			if v != "" {
				values = summaryValue(values, key, v)
			}
		case json.Number, bool:
			values = append(values, fmt.Sprintf("%v=%v", key, v))
		}
	}
	return ansi.Truncate(singleLine(strings.Join(values, " ")), toolRowArgumentWidth, "…")
}

// summaryValue appends a value, or puts it first when it is the call's path:
// a model may send the path after the other arguments.
func summaryValue(values []string, key any, value string) []string {
	if slices.Contains(values, value) {
		// The same value twice ("calc/calc.go calc/calc.go") says it once.
		return values
	}
	if key == "path" {
		return append([]string{value}, values...)
	}
	return append(values, value)
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

var modeField = regexp.MustCompile(`"mode":\s*"(create|replace)"`)

// localFileSummary reads a write as its path and mode and an edit as its
// path. On 14 October 2026 a replace read "local_write calc/calc.go
// 36dc0cc81d1ed…": the scalar summary listed expected_sha256, which says
// nothing to the person, and pushed the mode out of the row.
func localFileSummary(call root.ToolCall) (string, bool) {
	if call.Name != "local_write" && call.Name != "local_edit" {
		return "", false
	}
	raw := call.Arguments.Bytes()
	match := pathField.FindSubmatch(raw)
	if match == nil {
		return "", false
	}
	summary := string(match[1])
	if call.Name == "local_write" {
		if mode := modeField.FindSubmatch(raw); mode != nil {
			summary += " · " + string(mode[1])
		}
	}
	return ansi.Truncate(singleLine(summary), toolRowArgumentWidth, "…"), true
}

// stepDoneFields are the axlr_step_done fields that say what a step handed
// back, the first one sent naming the row: on 14 October 2026 a step that
// sent its scope and its draft path read "calc/calc.go calc/calc.go".
var stepDoneFields = []string{"summary", "root_cause", "observed", "proposed_fix", "scope", "draft_path", "expected", "criteria", "report"}

func stepDoneSummary(call root.ToolCall) (string, bool) {
	if call.Name != application.HostStepDoneName || len(call.Arguments.Bytes()) > 64<<10 {
		return "", false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(call.Arguments.Bytes(), &fields) != nil {
		return "", false
	}
	for _, name := range stepDoneFields {
		var value string
		if json.Unmarshal(fields[name], &value) == nil && strings.TrimSpace(value) != "" {
			return ansi.Truncate(singleLine(value), toolRowArgumentWidth, "…"), true
		}
	}
	if command, ok := execCommand(fields["check_command"]); ok {
		return ansi.Truncate(command, toolRowArgumentWidth, "…"), true
	}
	return "", false
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

// resultBytes is the size a row shows for a call's result: the bytes a
// local_read returned from the file, otherwise the result's own size. On
// 14 October 2026 a read of an 809-byte file showed 1.2 KB, the size of
// its JSON envelope.
func resultBytes(call root.ToolCall, content root.Text) int {
	if call.Name == "local_read" {
		var read struct {
			Output struct {
				ReturnedBytes *int `json:"returned_bytes"`
			} `json:"output"`
		}
		if json.Unmarshal([]byte(content), &read) == nil && read.Output.ReturnedBytes != nil {
			return *read.Output.ReturnedBytes
		}
	}
	return len(content)
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
		case strings.HasPrefix(content, "invalid tool invocation rejected: ") && notInStep(content):
			// On 14 October 2026 a write refused because a read-only step
			// does not offer it read "invalid arguments, the model retries".
			return "transcript.toolNotInStep"
		case strings.HasPrefix(content, "invalid tool invocation rejected: "):
			return "transcript.toolInvalid"
		case strings.HasPrefix(content, "unknown tool "):
			return "transcript.toolUnknown"
		case overCallBudget(content):
			return "transcript.toolOverBudget"
		}
	}
	return "transcript.toolDenied"
}

// notInStep reports a call refused because the ceremony step does not
// offer its tool, whatever its arguments.
func notInStep(content string) bool {
	return strings.Contains(content, " is not available in this step") || strings.Contains(content, " is not available during this ceremony step")
}

// overCallBudget reports a call the console did not run because the turn's
// tool-call budget was spent: on 10 October 2026 all 93 calls of a batch
// past the budget read "denied", as if the person had refused each one. A
// session saved before then carries the older refusal, "not run: the turn
// reached its 32 tool-call limit", which reads the same way.
func overCallBudget(content string) bool {
	return strings.HasPrefix(content, domain.OverBudgetOutcomePrefix) || strings.HasPrefix(content, "not run: the turn reached its ")
}
