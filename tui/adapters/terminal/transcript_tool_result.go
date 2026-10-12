package terminal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// maxExpandedResultLines bounds what a click opens under a tool row; Info
// (Ctrl+P → I) keeps the whole result.
const maxExpandedResultLines = 200

// maxReadableResultBytes is the largest result read as JSON; a larger one
// is shown as it is.
const maxReadableResultBytes = 128 << 10

// toolNoteKey identifies a tool row's expansion among the transcript's
// expanded notes.
func toolNoteKey(call root.ToolCallID) string { return "tool." + string(call) }

// toolResultRows lay out what a call returned under its row once a click
// opens it. On 12 October 2026 a row such as "✓ local_exec go test … · 374
// B" could not be opened; the only view was Info, which printed every
// result of the session as escaped JSON. A command shows its exit code,
// standard output and error as text, a read the text it read, a write or
// edit its diff from the recorded snapshots, and any other result its JSON
// with string values unescaped (multi-line strings as blocks). A denied or
// refused call shows what the model was told.
func toolResultRows(s domain.SessionState, call root.ToolCall, record *domain.PendingTool, result *root.Message, theme Theme) []transcriptRow {
	text := ""
	switch {
	case result != nil:
		text = string(result.Content)
	case record != nil && record.Outcome != nil:
		text = string(record.Outcome.Content)
	default:
		return []transcriptRow{expandedRow(theme.T("transcript.toolNoResult"), transcriptRowConsole)}
	}
	operation := string(call.Name)
	if tool, _, known, err := application.ResolveToolCall(s.ToolSnapshot, call); known && err == nil && tool.Identity.Kind == domain.ToolKindLocal {
		operation = "local_" + string(tool.Identity.LocalOperation)
	}
	if record != nil && record.Decision != domain.DecisionDeny && record.Outcome != nil && record.Outcome.Change != nil && (operation == "local_write" || operation == "local_edit") {
		if rows, ok := changeResultRows(*record.Outcome.Change, theme); ok {
			return capRows(rows, theme)
		}
	}
	var lines []string
	switch operation {
	case "local_exec":
		lines = execResultLines(text, theme)
	case "local_read":
		lines = readResultLines(text)
	}
	if lines == nil {
		lines = strings.Split(readableResult(text), "\n")
	}
	rows := make([]transcriptRow, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, expandedRow(line, transcriptRowPlain))
	}
	return capRows(rows, theme)
}

// expandedRow is one line of an opened result, hung under the tool row's
// text.
func expandedRow(text string, kind transcriptRowKind) transcriptRow {
	return transcriptRow{Label: "  ", Text: text, Kind: kind, Indent: true, Wrap: true}
}

// capRows keeps the first maxExpandedResultLines rows and says where the
// rest is.
func capRows(rows []transcriptRow, theme Theme) []transcriptRow {
	if hidden := len(rows) - maxExpandedResultLines; hidden > 0 {
		rows = append(rows[:maxExpandedResultLines:maxExpandedResultLines], expandedRow(theme.Tn("transcript.toolMoreLines", hidden, hidden), transcriptRowConsole))
	}
	return rows
}

// toolEnvelope is the local runtime's answer around a tool's output.
type toolEnvelope struct {
	Status     string          `json:"status"`
	DurationMS int64           `json:"duration_ms"`
	Output     json.RawMessage `json:"output"`
	Error      json.RawMessage `json:"error"`
}

func readEnvelope(text string) (toolEnvelope, bool) {
	var envelope toolEnvelope
	if len(text) > maxReadableResultBytes || json.Unmarshal([]byte(text), &envelope) != nil || envelope.Status == "" {
		return toolEnvelope{}, false
	}
	return envelope, true
}

// execResultLines reads a command's result as its exit code, then its
// standard output and error as text; nil when the result has another
// shape.
func execResultLines(text string, theme Theme) []string {
	envelope, ok := readEnvelope(text)
	if !ok {
		return nil
	}
	var output struct {
		ExitCode       *int   `json:"exit_code"`
		Stdout         string `json:"stdout"`
		Stderr         string `json:"stderr"`
		DiscardedBytes int64  `json:"discarded_bytes"`
		Truncated      bool   `json:"truncated"`
	}
	if len(envelope.Output) == 0 || string(envelope.Output) == "null" || json.Unmarshal(envelope.Output, &output) != nil || output.ExitCode == nil {
		return nil
	}
	head := theme.Tf("transcript.toolExit", *output.ExitCode)
	if envelope.DurationMS > 0 {
		head += " · " + formatDuration(envelope.DurationMS)
	}
	if output.Truncated || output.DiscardedBytes > 0 {
		head += " · " + theme.Tf("transcript.toolDiscarded", formatBytes(int(output.DiscardedBytes)))
	}
	lines := []string{head}
	for _, stream := range []struct{ label, text string }{{"stdout", output.Stdout}, {"stderr", output.Stderr}} {
		if strings.TrimSpace(stream.text) == "" {
			continue
		}
		lines = append(lines, stream.label+":")
		for _, line := range strings.Split(strings.TrimRight(stream.text, "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
	}
	if len(lines) == 1 {
		lines = append(lines, theme.T("transcript.toolNoOutput"))
	}
	if errLines := readableRaw(envelope.Error); errLines != "" {
		lines = append(lines, strings.Split("error:\n"+indentLines(errLines, "  "), "\n")...)
	}
	return lines
}

// readResultLines is the text a read returned; nil for another shape.
func readResultLines(text string) []string {
	envelope, ok := readEnvelope(text)
	if !ok {
		return nil
	}
	var output struct {
		Content *string `json:"content"`
	}
	if len(envelope.Output) == 0 || json.Unmarshal(envelope.Output, &output) != nil || output.Content == nil {
		return nil
	}
	return strings.Split(strings.TrimRight(*output.Content, "\n"), "\n")
}

// changeResultRows draw a recorded write or edit as its diff, removed lines
// red and added ones green, from the snapshots Ctrl+D shows.
func changeResultRows(change domain.FileChange, theme Theme) ([]transcriptRow, bool) {
	if change.Unavailable != "" {
		return []transcriptRow{expandedRow(theme.T("changes."+change.Unavailable), transcriptRowConsole)}, true
	}
	preview := previewChange(change)
	if len(preview.Lines) == 0 {
		return nil, false
	}
	newFile := change.Created && change.Before == ""
	rows := []transcriptRow{expandedRow("── "+string(change.Path)+" ──", transcriptRowSpeaker)}
	for _, line := range preview.Lines {
		switch {
		case line.Heading:
			rows = append(rows, expandedRow(hunkHeading(line, newFile, theme), transcriptRowConsole))
		case line.NoNewline:
			rows = append(rows, expandedRow("\\ "+theme.T("changes.noNewline"), transcriptRowConsole))
		case line.Kind == udiff.Delete:
			row := expandedRow(line.Text, transcriptRowDiffRemoved)
			row.Label, row.LabelTone = "  - ", toneError
			rows = append(rows, row)
		case line.Kind == udiff.Insert:
			row := expandedRow(line.Text, transcriptRowDiffAdded)
			row.Label, row.LabelTone = "  + ", toneGood
			rows = append(rows, row)
		default:
			row := expandedRow(line.Text, transcriptRowPlain)
			row.Label = "    "
			rows = append(rows, row)
		}
	}
	return rows, true
}

// readableResult is a tool result as a person reads it: JSON as an indented
// outline whose string values are unescaped, a multi-line string as a
// block under its key; any other text as it is.
func readableResult(text string) string {
	if len(text) > maxReadableResultBytes {
		return text
	}
	if readable := readableRaw([]byte(text)); readable != "" {
		return readable
	}
	return text
}

// readableRaw is readableResult for JSON only: "" for null, empty or
// invalid JSON.
func readableRaw(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" || !json.Valid(trimmed) {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	value, err := decodeOrdered(decoder)
	if err != nil {
		return ""
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ""
	}
	var lines []string
	writeReadable(&lines, value, "")
	return strings.Join(lines, "\n")
}

// orderedField keeps an object's fields in the order the JSON gave them.
type orderedField struct {
	key   string
	value any
}

type orderedObject []orderedField

func decodeOrdered(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := orderedObject{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			value, err := decodeOrdered(decoder)
			if err != nil {
				return nil, err
			}
			object = append(object, orderedField{key: fmt.Sprint(key), value: value})
		}
		_, err = decoder.Token()
		return object, err
	case json.Delim('['):
		var array []any
		for decoder.More() {
			value, err := decodeOrdered(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err = decoder.Token()
		if array == nil {
			array = []any{}
		}
		return array, err
	}
	return token, nil
}

// writeReadable adds a value's lines at an indent: a scalar on one line,
// an object as "key: value" lines, an array as "- item" lines, a
// multi-line string as its lines.
func writeReadable(lines *[]string, value any, indent string) {
	switch v := value.(type) {
	case orderedObject:
		if len(v) == 0 {
			*lines = append(*lines, indent+"{}")
		}
		for _, field := range v {
			writeField(lines, field.key+":", field.value, indent)
		}
	case []any:
		if len(v) == 0 {
			*lines = append(*lines, indent+"[]")
		}
		for _, item := range v {
			writeField(lines, "-", item, indent)
		}
	default:
		for _, line := range strings.Split(scalarText(v), "\n") {
			*lines = append(*lines, indent+line)
		}
	}
}

// writeField writes one key or list item: a one-line scalar after it, and
// anything else below it, indented.
func writeField(lines *[]string, lead string, value any, indent string) {
	switch v := value.(type) {
	case orderedObject:
		if len(v) == 0 {
			*lines = append(*lines, indent+lead+" {}")
			return
		}
	case []any:
		if len(v) == 0 {
			*lines = append(*lines, indent+lead+" []")
			return
		}
	default:
		if text := scalarText(v); !strings.Contains(text, "\n") {
			*lines = append(*lines, indent+lead+" "+text)
			return
		}
		*lines = append(*lines, indent+lead)
		writeReadable(lines, strings.TrimRight(scalarText(v), "\n"), indent+"  ")
		return
	}
	*lines = append(*lines, indent+lead)
	writeReadable(lines, value, indent+"  ")
}

func scalarText(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func indentLines(text, indent string) string {
	return indent + strings.ReplaceAll(text, "\n", "\n"+indent)
}
