package application

import (
	"encoding/json"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
)

// turnRecord is what a person's turn left outside the transcript: the files
// it wrote, which are on disk, and the memory it recorded, which is in KMP.
// A checkpoint carries it for each omitted turn so the model looks there
// before paging the turn's messages back.
type turnRecord struct {
	files  []string
	memory []map[string]any
	// worked says the turn wrote a file or made several local calls, the
	// same measure the memory reminder uses; unrecorded says it did so in a
	// session that uses KMP without an accepted memory write.
	worked, unrecorded bool
}

func (r turnRecord) fields() map[string]any {
	fields := map[string]any{}
	if len(r.files) > 0 {
		fields["files_written"] = r.files
	}
	if len(r.memory) > 0 {
		fields["memory_written"] = r.memory
	}
	if r.unrecorded {
		fields["unrecorded"] = true
	}
	return fields
}

// omittedTurnRecords maps the index of each person's message in [first, cut)
// to what its turn left behind. Console messages, which start with "[AXLR",
// belong to the request before them, as the memory reminder counts them: a
// write the model makes after the reminder is the reminded turn's write.
func omittedTurnRecords(original []root.Message, first, cut int) map[int]turnRecord {
	results := map[root.ToolCallID]string{}
	usesMemory := false
	for _, message := range original {
		if message.Role == root.RoleTool {
			results[message.ToolCallID] = string(message.Content)
		}
		for _, call := range message.ToolCalls {
			if name, _ := memoryCall(call); strings.HasPrefix(name, "kmp_") {
				usesMemory = true
			}
		}
	}
	records := map[int]turnRecord{}
	turn := -1
	for i := first; i < cut; i++ {
		message := original[i]
		if message.Role == root.RoleUser && !strings.HasPrefix(string(message.Content), "[AXLR") {
			turn = i
			records[turn] = turnRecord{}
			continue
		}
		if turn < 0 || message.Role != root.RoleAssistant {
			continue
		}
		record := records[turn]
		for _, call := range message.ToolCalls {
			local := strings.HasPrefix(string(call.Name), "local_")
			if local {
				record.worked = record.worked || call.Name == "local_write" || call.Name == "local_edit"
			}
			switch call.Name {
			case "local_write", "local_edit":
				if path := argumentString(call.Arguments, "path"); path != "" && !contains(record.files, path) {
					record.files = append(record.files, path)
				}
			}
			if name, arguments := memoryCall(call); name == "kmp_write_memory" && memoryWriteAccepted(results[call.ID]) {
				written := map[string]any{}
				for _, key := range []string{"about", "idempotency_key"} {
					if value := argumentString(arguments, key); value != "" {
						written[key] = value
					}
				}
				record.memory = append(record.memory, written)
			}
		}
		records[turn] = record
	}
	for index, record := range records {
		if !record.worked && localCalls(original, index, cut) >= memoryReminderCalls {
			record.worked = true
		}
		record.unrecorded = usesMemory && record.worked && len(record.memory) == 0
		records[index] = record
	}
	return records
}

// memoryCall is the plugin tool a call reaches, directly or through the
// bridge, with its arguments.
func memoryCall(call root.ToolCall) (string, root.JSONValue) {
	if call.Name != HostCallToolName {
		return string(call.Name), call.Arguments
	}
	var bridged struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(call.Arguments.Bytes(), &bridged) != nil {
		return "", call.Arguments
	}
	arguments, err := root.NewJSONObject(bridged.Arguments)
	if err != nil {
		return bridged.Name, call.Arguments
	}
	return bridged.Name, arguments
}

// memoryWriteAccepted reads a write's result: a runtime envelope without an
// error whose output is not an error. A denied call's plain text is not.
func memoryWriteAccepted(result string) bool {
	value, err := decodeContextJSON([]byte(result))
	if err != nil {
		return false
	}
	envelope, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if envelope["error"] != nil {
		return false
	}
	if output, ok := envelope["output"].(map[string]any); ok && output["is_error"] == true {
		return false
	}
	return true
}

func argumentString(arguments root.JSONValue, key string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &fields) != nil {
		return ""
	}
	var value string
	if json.Unmarshal(fields[key], &value) != nil {
		return ""
	}
	return value
}

// localCalls counts the local tool calls of the person's turn that starts at
// index, console messages included, up to end.
func localCalls(original []root.Message, index, end int) int {
	count := 0
	for i := index + 1; i < end; i++ {
		if original[i].Role == root.RoleUser && !strings.HasPrefix(string(original[i].Content), "[AXLR") {
			break
		}
		for _, call := range original[i].ToolCalls {
			if strings.HasPrefix(string(call.Name), "local_") {
				count++
			}
		}
	}
	return count
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
