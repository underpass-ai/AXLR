package application

import (
	"encoding/json"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
)

// closedWriteArgumentBytes is the size from which a closed turn's
// local_write or local_edit arguments are shortened: smaller ones are kept
// exact, since the shortened form would save little.
const closedWriteArgumentBytes = 512

// The notes of a shortened write: a closed turn's, and the turn in
// progress's once it no longer fits the model context.
const (
	closedWriteRecover    = "The turn has closed and the file on disk holds what this call wrote: local_read it."
	compactedWriteRecover = "This turn no longer fits the model context, so this call's text was left out; the file on disk holds what it wrote: local_read it."
)

// omittedKey holds what a shortened write left out. It is no parameter of
// any tool, so the shortened arguments read as a record of a call, not as
// a call to copy: on 14 October 2026 glm-5.3-flash, in /plan, sent
// local_write twice with the earlier form ("content_omitted_bytes":"866",
// "recover":"…", expected_sha256 and no content), and both failed.
const omittedKey = "[omitted]"

// closedWriteArguments shortens the arguments of a local_write or local_edit
// call of a closed turn whose result says it succeeded: the text it wrote is
// on disk, so the projection keeps the path and the mode and says how many
// bytes of each text were left out, under omittedKey with a note that a new
// call needs its full text. Session c33e8e86 carried 72 KB of such
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
	omitted := map[string]any{"note": recover + " This is a record of the call, not arguments to send again: a new " + string(call.Name) + " needs its full " + strings.Join(texts, " and ") + "."}
	for _, text := range texts {
		var content string
		if json.Unmarshal(fields[text], &content) == nil {
			omitted[text+"_bytes"] = len(content)
		}
	}
	// The path and the mode say what was written; the digest and the why
	// are left out with the text, so nothing in the record is a call's
	// complete argument set.
	shortened := map[string]any{omittedKey: omitted}
	for _, key := range []string{"path", "mode"} {
		if value, ok := fields[key]; ok {
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
// envelope that classifyOutcome reads as completed. A denied, cancelled,
// refused or uncertain call's plain text is not, nor is a failure.
func writeSucceeded(result string) bool {
	if outcome, _ := classifyOutcome(result); outcome != turnCompleted {
		return false
	}
	_, err := decodeContextJSON([]byte(result))
	return err == nil && strings.HasPrefix(strings.TrimSpace(result), "{")
}
