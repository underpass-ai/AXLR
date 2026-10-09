package application

import (
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
)

// closedSchemaBytes is the size from which a closed turn's exact schema is
// left out of the projection, as closedWriteArguments does for writes.
const closedSchemaBytes = 512

// closedSchemaRecover is the note in place of the schema.
const closedSchemaRecover = "The turn has closed and this exact schema was left out of the context; call axlr_tools with this name again if you need it."

// closedSchemaResult shortens the result of an axlr_tools call of a closed
// turn that returned one exact schema (called with name): it kept 7.4 KB of
// schemas in the history of the 2026-10-08 GLM session for good, read once
// to write one memory. Searches and lists are kept, being short; the saved
// transcript keeps the schema whole and the turn in progress keeps it.
func closedSchemaResult(call root.ToolCall, result string) (root.Text, bool) {
	if len(result) < closedSchemaBytes {
		return "", false
	}
	var args struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if json.Unmarshal(call.Arguments.Bytes(), &args) != nil || args.Name == "" {
		return "", false
	}
	var failure struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(result), &failure) == nil && failure.Error != "" {
		return "", false // a refusal says what went wrong: keep it
	}
	stub := map[string]any{"name": args.Name, "schema_omitted_bytes": len(result), "recover": closedSchemaRecover}
	if args.Path != "" {
		stub["path"] = args.Path
	}
	encoded, err := json.Marshal(stub)
	if err != nil {
		return "", false
	}
	return root.Text(encoded), true
}
