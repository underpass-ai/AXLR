package terminal

import (
	"bytes"
	"encoding/json"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
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
		Name        string                   `json:"name"`
		Description string                   `json:"description"`
		InputSchema json.RawMessage          `json:"input_schema"`
		Program     string                   `json:"program"`
		Args        []string                 `json:"args"`
		Files       []application.ForgedFile `json:"files"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Name == "" || len(call.Files) == 0 {
		return "", false
	}
	return forgeCardOf(call.Program, call.Args, call.Description, call.InputSchema, call.Files), true
}

// forgeCardOf lays a tool out: the command, the description, the schema
// and each file as code. The install card adds its origin above it.
func forgeCardOf(program string, args []string, description string, inputSchema json.RawMessage, files []application.ForgedFile) string {
	var b strings.Builder
	b.WriteString("$ " + formatCommand(program, args) + "\n\n")
	b.WriteString(description + "\n\n")
	var schema bytes.Buffer
	if json.Indent(&schema, inputSchema, "", "  ") == nil {
		b.WriteString("input_schema:\n" + schema.String() + "\n")
	}
	for _, file := range files {
		b.WriteString("\n── " + file.Path + " ──\n")
		b.WriteString(strings.TrimRight(file.Content, "\n") + "\n")
	}
	return b.String()
}

// savesAlwaysAllow reports whether "always allow" can save a rule for the
// pending call: host tools have none, so the card does not offer it.
func savesAlwaysAllow(snapshot []domain.AvailableTool, p domain.PendingTool) bool {
	tool, _, known, err := application.ResolveToolCall(snapshot, p.Call)
	return known && err == nil && tool.Identity.Kind != domain.ToolKindHost
}

// publishApprovalCard says what an axlr_publish_tool approval sends and where,
// with the model's why first: a publication leaves the workspace, so the
// person reads it before the upload, autonomy included.
func publishApprovalCard(arguments root.JSONValue, theme Theme) string {
	var call struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Catalog string `json:"catalog"`
	}
	_ = json.Unmarshal(arguments.Bytes(), &call)
	what := call.Name + "@" + call.Version
	if call.Version == "" {
		what = call.Name + " (" + theme.T("approval.publishDefaultVersion") + ")"
	}
	where := call.Catalog
	if where == "" {
		where = theme.T("approval.publishDefaultCatalog")
	}
	text := theme.Tf("approval.publishCard", what, where)
	if why := application.CallWhy(arguments); why != "" {
		text = theme.Tf("approval.why", strings.ReplaceAll(why, "\n", " ")) + "\n\n" + text
	}
	return text
}
