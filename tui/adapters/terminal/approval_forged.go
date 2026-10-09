package terminal

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// forgedName is the tool a forge or run card is about, read from the call's
// arguments; the card shows "?" for arguments that do not name one.
func forgedName(arguments []byte) string {
	var call struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Name == "" {
		return "?"
	}
	return call.Name
}

// forgeCard lays an axlr_forge_tool call out for review: the command, the
// description and the schema, then each file as the code it is rather than
// a JSON string with escaped newlines (seen on 9 Oct 2026, a Python file
// was one line of \n escapes). ok is false when the arguments are not a
// forge call's shape; the card then shows them as JSON.
func forgeCard(arguments []byte) (string, bool) {
	var call struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
		Program     string          `json:"program"`
		Args        []string        `json:"args"`
		Files       []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Name == "" || len(call.Files) == 0 {
		return "", false
	}
	var b strings.Builder
	b.WriteString("$ " + strings.Join(append([]string{call.Program}, call.Args...), " ") + "\n\n")
	b.WriteString(call.Description + "\n\n")
	var schema bytes.Buffer
	if json.Indent(&schema, call.InputSchema, "", "  ") == nil {
		b.WriteString("input_schema:\n" + schema.String() + "\n")
	}
	for _, file := range call.Files {
		b.WriteString("\n── " + file.Path + " ──\n")
		b.WriteString(strings.TrimRight(file.Content, "\n") + "\n")
	}
	return b.String(), true
}

// savesAlwaysAllow reports whether "always allow" can save a rule for the
// pending call: host tools have none, so the card does not offer it.
func savesAlwaysAllow(snapshot []domain.AvailableTool, p domain.PendingTool) bool {
	tool, _, known, err := application.ResolveToolCall(snapshot, p.Call)
	return known && err == nil && tool.Identity.Kind != domain.ToolKindHost
}
