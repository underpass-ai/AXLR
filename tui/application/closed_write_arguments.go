package application

import (
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
)

// closedWriteArgumentBytes is the size from which a closed turn's
// local_write or local_edit arguments are shortened: smaller ones are kept
// exact, since the shortened form would save little.
const closedWriteArgumentBytes = 512

// closedWriteArguments shortens the arguments of a local_write or local_edit
// call of a closed turn: the text it wrote is on disk, so the projection keeps
// the path and the mode and says how many bytes of each text were left out.
// Session c33e8e86 carried 72 KB of such arguments in its last request, the
// largest part of the history the projection never shortened. The saved
// transcript keeps them whole; other calls are returned unchanged.
func closedWriteArguments(call root.ToolCall) root.JSONValue {
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
	if len(raw) < closedWriteArgumentBytes {
		return call.Arguments
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return call.Arguments
	}
	shortened := map[string]any{"recover": "The turn has closed and the file on disk holds what this call wrote: local_read it."}
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
