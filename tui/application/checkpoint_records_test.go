package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func recordCall(t *testing.T, id, name, arguments string) root.ToolCall {
	t.Helper()
	return root.ToolCall{ID: root.ToolCallID(id), Name: root.ToolName(name), Arguments: hostJSON(t, arguments)}
}

const acceptedWrite = `{"protocol_version":1,"tool":"kmp_write_memory","status":"ok","error":null,"output":{"content":[{"type":"text","text":"Ingested 1 entry."}],"is_error":false}}`

// A session with KMP: the first turn writes a file and records memory, the
// second runs five local commands and records nothing, the third records
// after the console's reminder, which belongs to it. Padding closes them.
func recordedTranscript(t *testing.T, memory bool) []root.Message {
	t.Helper()
	write := `{"name":"kmp_write_memory","arguments":{"about":"project:AXLR","idempotency_key":"k-1","text":"decided"}}`
	messages := []root.Message{
		{Role: root.RoleUser, Content: "escribe a.go"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "w1", "local_write", `{"path":"a.go","content":"package a","mode":"create"}`), recordCall(t, "w2", "local_edit", `{"path":"b.go","old_text":"x","new_text":"y"}`)}},
		{Role: root.RoleTool, ToolCallID: "w1", Content: `{"written":true}`},
		{Role: root.RoleTool, ToolCallID: "w2", Content: `{"written":true}`},
	}
	if memory {
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "m1", string(HostCallToolName), write)}},
			root.Message{Role: root.RoleTool, ToolCallID: "m1", Content: acceptedWrite})
	}
	messages = append(messages, root.Message{Role: root.RoleAssistant, Content: root.Text("hecho " + strings.Repeat("x", 3000))})
	messages = append(messages, root.Message{Role: root.RoleUser, Content: "ejecuta cinco cosas"})
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("e%d", i)
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, id, "local_exec", `{"program":"true"}`)}},
			root.Message{Role: root.RoleTool, ToolCallID: root.ToolCallID(id), Content: `{"exit":0}`})
	}
	messages = append(messages, root.Message{Role: root.RoleAssistant, Content: root.Text("listo " + strings.Repeat("x", 3000))})
	messages = append(messages,
		root.Message{Role: root.RoleUser, Content: "cambia c.go"},
		root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "w3", "local_write", `{"path":"c.go","content":"c","mode":"create"}`)}},
		root.Message{Role: root.RoleTool, ToolCallID: "w3", Content: `{"written":true}`},
		root.Message{Role: root.RoleAssistant, Content: "cambiado"},
		root.Message{Role: root.RoleUser, Content: root.Text(memoryReminder)})
	if memory {
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "m2", string(HostCallToolName), strings.Replace(write, "k-1", "k-2", 1))}},
			root.Message{Role: root.RoleTool, ToolCallID: "m2", Content: acceptedWrite},
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "m3", string(HostCallToolName), strings.Replace(write, "k-1", "k-denied", 1))}},
			root.Message{Role: root.RoleTool, ToolCallID: "m3", Content: "denied"})
	}
	messages = append(messages, root.Message{Role: root.RoleAssistant, Content: root.Text("anotado " + strings.Repeat("x", 3000))})
	for i := 0; i < 6; i++ {
		messages = append(messages, root.Message{Role: root.RoleUser, Content: root.Text(fmt.Sprintf("relleno %d", i))}, root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("relleno ", 1500))})
	}
	return messages
}

type checkpointInputs struct {
	Users []struct {
		Index      int              `json:"message_index"`
		Content    string           `json:"quoted_content"`
		Files      []string         `json:"files_written"`
		Memory     []map[string]any `json:"memory_written"`
		Unrecorded bool             `json:"unrecorded"`
	} `json:"historical_user_inputs_newest_first"`
	Excerpts       []any  `json:"excerpts_newest_first"`
	Retrieval      string `json:"retrieval"`
	RecordsOmitted *int   `json:"records_omitted_at_or_before"`
}

func projectRecorded(t *testing.T, messages []root.Message) checkpointInputs {
	t.Helper()
	budget, err := domain.NewContextBudget(40<<10, 20<<10, 4<<10, 4<<10)
	if err != nil {
		t.Fatal(err)
	}
	projector, _ := NewModelContextProjector(budget)
	projection, err := projector.Project(messages)
	if err != nil || projection.CutIndex == 0 {
		t.Fatalf("checkpoint did not trigger: err=%v cut=%d", err, projection.CutIndex)
	}
	var checkpoint checkpointInputs
	if err := json.Unmarshal([]byte(projection.Messages[0].Content), &checkpoint); err != nil {
		t.Fatal(err)
	}
	if modelMessageBytes(projection.Messages[0]) > budget.CheckpointBytes() {
		t.Fatal("checkpoint exceeded its budget")
	}
	return checkpoint
}

func TestCheckpointMapsWhatEachOmittedTurnLeftBehind(t *testing.T) {
	checkpoint := projectRecorded(t, recordedTranscript(t, true))
	byContent := map[string]int{}
	for i, input := range checkpoint.Users {
		byContent[input.Content] = i
	}
	first := checkpoint.Users[byContent["escribe a.go"]]
	if strings.Join(first.Files, ",") != "a.go,b.go" || len(first.Memory) != 1 || first.Memory[0]["about"] != "project:AXLR" || first.Memory[0]["idempotency_key"] != "k-1" || first.Unrecorded {
		t.Fatalf("first turn: %+v", first)
	}
	second := checkpoint.Users[byContent["ejecuta cinco cosas"]]
	if len(second.Files) != 0 || len(second.Memory) != 0 || !second.Unrecorded {
		t.Fatalf("second turn: %+v", second)
	}
	// The write after the console's reminder belongs to the reminded turn,
	// and the denied one does not count.
	third := checkpoint.Users[byContent["cambia c.go"]]
	if strings.Join(third.Files, ",") != "c.go" || len(third.Memory) != 1 || third.Memory[0]["idempotency_key"] != "k-2" || third.Unrecorded {
		t.Fatalf("third turn: %+v", third)
	}
	reminder := checkpoint.Users[byContent[memoryReminder]]
	if len(reminder.Files) != 0 || len(reminder.Memory) != 0 || reminder.Unrecorded {
		t.Fatalf("console message carries a record: %+v", reminder)
	}
	if len(checkpoint.Excerpts) != 0 || !strings.Contains(checkpoint.Retrieval, "kmp_ask") || checkpoint.RecordsOmitted != nil {
		t.Fatalf("checkpoint shape: %+v", checkpoint)
	}
}

// Without KMP in the session nothing is flagged as unrecorded.
func TestCheckpointFlagsNothingUnrecordedWithoutMemory(t *testing.T) {
	checkpoint := projectRecorded(t, recordedTranscript(t, false))
	for _, input := range checkpoint.Users {
		if input.Unrecorded || len(input.Memory) != 0 {
			t.Fatalf("flagged without KMP: %+v", input)
		}
		if input.Content == "escribe a.go" && strings.Join(input.Files, ",") != "a.go,b.go" {
			t.Fatalf("files lost without KMP: %+v", input)
		}
	}
}

// files_written tells the model the file on disk holds the write: a write
// the person denied, the mode refused or the runtime failed is not listed.
func TestCheckpointListsOnlyTheWritesThatSucceeded(t *testing.T) {
	write := func(id, path string) root.ToolCall {
		return recordCall(t, id, "local_write", `{"path":"`+path+`","content":"package x","mode":"create"}`)
	}
	original := []root.Message{
		{Role: root.RoleUser, Content: "write them"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{write("w1", "denied.go"), write("w2", "failed.go"), write("w3", "refused.go"), write("w4", "uncertain.go"), write("w5", "written.go")}},
		{Role: root.RoleTool, ToolCallID: "w1", Content: "tool call denied by user"},
		{Role: root.RoleTool, ToolCallID: "w2", Content: `{"protocol_version":1,"tool":"write","status":"failed","error":{"code":"io_error","message":"read-only file system"}}`},
		{Role: root.RoleTool, ToolCallID: "w3", Content: "invalid tool invocation rejected: the review mode does not write files"},
		{Role: root.RoleTool, ToolCallID: "w4", Content: "tool execution failed; effect unknown: context canceled"},
		{Role: root.RoleTool, ToolCallID: "w5", Content: `{"protocol_version":1,"tool":"write","status":"completed","error":null,"output":{"bytes":9}}`},
		{Role: root.RoleAssistant, Content: "one written"},
		{Role: root.RoleUser, Content: "next"},
	}
	files, _ := omittedTurnRecords(original, 0, 8)[0].fields()["files_written"].([]string)
	if strings.Join(files, ",") != "written.go" {
		t.Fatalf("files_written = %v", files)
	}
}

// A turn that only read files did no work worth recording, as the memory
// reminder counts it, even in a session that uses KMP.
func TestCheckpointDoesNotFlagAReadOnlyTurn(t *testing.T) {
	messages := []root.Message{
		{Role: root.RoleUser, Content: "lista los encabezados"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "k1", string(HostCallToolName), `{"name":"kmp_wake","arguments":{}}`)}},
		{Role: root.RoleTool, ToolCallID: "k1", Content: `{"status":"ok"}`},
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("r%d", i)
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, id, "local_read", `{"path":"docs/console.md"}`)}},
			root.Message{Role: root.RoleTool, ToolCallID: root.ToolCallID(id), Content: `{"content":""}`})
	}
	messages = append(messages, root.Message{Role: root.RoleAssistant, Content: "encabezados"})
	if record := omittedTurnRecords(messages, 0, len(messages))[0]; record.worked || record.unrecorded {
		t.Fatalf("read-only turn: %+v", record)
	}
}

// Searching and listing change nothing, so a turn of them alone is not
// flagged unrecorded either.
func TestCheckpointDoesNotFlagASearchOnlyTurn(t *testing.T) {
	messages := []root.Message{
		{Role: root.RoleUser, Content: "dónde se define el presupuesto"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, "k1", string(HostCallToolName), `{"name":"kmp_wake","arguments":{}}`)}},
		{Role: root.RoleTool, ToolCallID: "k1", Content: `{"status":"ok"}`},
	}
	for i := 0; i < 8; i++ {
		id, name := fmt.Sprintf("s%d", i), "local_search"
		if i%2 == 1 {
			name = "local_list"
		}
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, id, name, `{"pattern":"budget"}`)}},
			root.Message{Role: root.RoleTool, ToolCallID: root.ToolCallID(id), Content: `{"matches":[]}`})
	}
	messages = append(messages, root.Message{Role: root.RoleAssistant, Content: "en context_budget.go"})
	if record := omittedTurnRecords(messages, 0, len(messages))[0]; record.worked || record.unrecorded {
		t.Fatalf("search-only turn: %+v", record)
	}
}
