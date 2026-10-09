package application

import (
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
)

// closedWriteArgumentBytes is the size from which a closed turn's
// local_write or local_edit arguments are shortened: smaller ones are kept
// exact, since the shortened form would save little.
const closedWriteArgumentBytes = 512

// The recover notes of a shortened write: a closed turn's, and the turn in
// progress's once it no longer fits the model context.
const (
	closedWriteRecover    = "The turn has closed and the file on disk holds what this call wrote: local_read it."
	compactedWriteRecover = "This turn no longer fits the model context, so this call's text was left out; the file on disk holds what it wrote: local_read it."
)

// closedWriteArguments shortens the arguments of a local_write or local_edit
// call of a closed turn whose result says it succeeded: the text it wrote is
// on disk, so the projection keeps the path and the mode and says how many
// bytes of each text were left out. Session c33e8e86 carried 72 KB of such
// arguments in its last request, the largest part of the history the
// projection never shortened. The saved transcript keeps them whole; other
// calls, and writes that were denied or failed, are returned unchanged. A
// compacted turn in progress shortens its own writes the same way, with
// recover saying why.
func closedWriteArguments(call root.ToolCall, result, recover string) root.JSONValue {
	var texts []string
	switch call.Name {
	case "local_write":
		texts = []string{"content"}
	case "local_edit":
		texts = []string{"old_text", "new_text"}
	default:
		return call.Arguments
	}
	raw := call.Arguments.Bytes()
	if len(raw) < closedWriteArgumentBytes || !writeSucceeded(result) {
		return call.Arguments
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return call.Arguments
	}
	shortened := map[string]any{"recover": recover}
	for key, value := range fields {
		omitted := false
		for _, text := range texts {
			if key == text {
				var content string
				if json.Unmarshal(value, &content) == nil {
					shortened[key+"_omitted_bytes"] = len(content)
					omitted = true
				}
			}
		}
		if !omitted {
			shortened[key] = value
		}
	}
	encoded, err := json.Marshal(shortened)
	if err != nil || len(encoded) >= len(raw) {
		return call.Arguments
	}
	arguments, err := root.NewJSONObject(encoded)
	if err != nil {
		return call.Arguments
	}
	return arguments
}

// writeSucceeded reads a local_write or local_edit result: a runtime
// envelope without an error whose status, when present, is completed. A
// denied, refused or uncertain call's plain text is not, nor is a failure.
func writeSucceeded(result string) bool {
	value, err := decodeContextJSON([]byte(result))
	if err != nil {
		return false
	}
	envelope, ok := value.(map[string]any)
	if !ok || envelope["error"] != nil {
		return false
	}
	status, present := envelope["status"]
	return !present || status == "completed"
}
