package application

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestProjectionShortensWriteArgumentsOfClosedTurnsOnly(t *testing.T) {
	content := strings.Repeat("línea de código\n", 200)
	write := recordCall(t, "w1", "local_write", `{"path":"a.go","content":`+quoteJSON(content)+`,"mode":"create"}`)
	edit := recordCall(t, "e1", "local_edit", `{"path":"b.go","old_text":`+quoteJSON(content)+`,"new_text":"x"}`)
	small := recordCall(t, "w2", "local_write", `{"path":"c.go","content":"package c","mode":"create"}`)
	read := recordCall(t, "r1", "local_read", `{"path":`+quoteJSON(strings.Repeat("d/", 400)+"a.go")+`}`)
	current := recordCall(t, "w3", "local_write", `{"path":"d.go","content":`+quoteJSON(content)+`,"mode":"replace"}`)
	original := []root.Message{
		{Role: root.RoleUser, Content: "escribe"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{write, edit, small, read}},
		{Role: root.RoleTool, ToolCallID: "w1", Content: `{"written":true}`},
		{Role: root.RoleTool, ToolCallID: "e1", Content: `{"written":true}`},
		{Role: root.RoleTool, ToolCallID: "w2", Content: `{"written":true}`},
		{Role: root.RoleTool, ToolCallID: "r1", Content: `{"text":"a"}`},
		{Role: root.RoleAssistant, Content: "hecho"},
		{Role: root.RoleUser, Content: "otra"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{current}},
		{Role: root.RoleTool, ToolCallID: "w3", Content: `{"written":true}`},
	}
	saved := string(original[1].ToolCalls[0].Arguments.Bytes())
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	calls := projection.Messages[1].ToolCalls
	var shortened map[string]any
	if err := json.Unmarshal(calls[0].Arguments.Bytes(), &shortened); err != nil {
		t.Fatal(err)
	}
	// The shortened form is a record under one key that is no tool
	// parameter, so it does not read as a call to copy (14 October 2026:
	// glm-5.3-flash sent the earlier form back as local_write arguments).
	omitted, _ := shortened[omittedKey].(map[string]any)
	if len(shortened) != 3 || shortened["path"] != "a.go" || shortened["mode"] != "create" || omitted["content_bytes"] != float64(len(content)) || !strings.Contains(omitted["note"].(string), "local_read") || !strings.Contains(omitted["note"].(string), "a new local_write needs its full content") {
		t.Fatalf("closed write: %s", calls[0].Arguments.Bytes())
	}
	shortened = nil
	if err := json.Unmarshal(calls[1].Arguments.Bytes(), &shortened); err != nil {
		t.Fatal(err)
	}
	omitted, _ = shortened[omittedKey].(map[string]any)
	if len(shortened) != 2 || shortened["path"] != "b.go" || omitted["old_text_bytes"] != float64(len(content)) || omitted["new_text_bytes"] != float64(1) || !strings.Contains(omitted["note"].(string), "full old_text and new_text") {
		t.Fatalf("closed edit: %s", calls[1].Arguments.Bytes())
	}
	for _, arguments := range []string{string(calls[0].Arguments.Bytes()), string(calls[1].Arguments.Bytes())} {
		if strings.Contains(arguments, `"recover"`) || strings.Contains(arguments, "_omitted_bytes") || strings.Contains(arguments, "expected_sha256") {
			t.Fatalf("the record keeps a call's shape: %s", arguments)
		}
	}
	// A small write and other tools stay exact, as does the current turn.
	if string(calls[2].Arguments.Bytes()) != string(small.Arguments.Bytes()) || string(calls[3].Arguments.Bytes()) != string(read.Arguments.Bytes()) {
		t.Fatal("a small write or another tool was shortened")
	}
	if string(projection.Messages[8].ToolCalls[0].Arguments.Bytes()) != string(current.Arguments.Bytes()) {
		t.Fatal("the current turn's write was shortened")
	}
	if string(original[1].ToolCalls[0].Arguments.Bytes()) != saved {
		t.Fatal("the saved transcript was modified")
	}
}

// The shortened form says the file on disk holds what the call wrote: a
// closed turn's write that was denied or failed keeps its arguments, the
// only copy of that text.
func TestProjectionKeepsTheArgumentsOfAWriteThatDidNotHappen(t *testing.T) {
	content := strings.Repeat("línea de código\n", 200)
	results := []root.Text{"tool call denied by user", `{"protocol_version":1,"tool":"write","status":"failed","error":{"code":"io_error","message":"disk full"}}`,
		`{"protocol_version":1,"tool":"write","status":"rejected","error":{"code":"invalid_arguments","message":"replace requires expected_sha256: pass the content_sha256 of a local_read of a.go"}}`,
		"invalid tool invocation rejected: local_write: replace requires expected_sha256: pass the content_sha256 of a local_read of a.go",
		cancelledByPerson, "tool execution failed; effect unknown: boom"}
	for _, result := range results {
		write := recordCall(t, "w1", "local_write", `{"path":"a.go","content":`+quoteJSON(content)+`,"mode":"create"}`)
		original := []root.Message{
			{Role: root.RoleUser, Content: "escribe"},
			{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{write}},
			{Role: root.RoleTool, ToolCallID: "w1", Content: result},
			{Role: root.RoleAssistant, Content: "no se pudo"},
			{Role: root.RoleUser, Content: "otra"},
		}
		projection, err := NewDefaultModelContextProjector().Project(original)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(projection.Messages[1].ToolCalls[0].Arguments.Bytes()); got != string(write.Arguments.Bytes()) || strings.Contains(got, omittedKey) || strings.Contains(got, `"recover"`) {
			t.Fatalf("%.30q: a write that did not happen was shortened to %s", result, got)
		}
	}
}

// Only the person's next request closes a turn. A console message (memory
// reminder, Jev, step reminder) or a message steered into the running turn
// does not: shortening the writes before it would rewrite messages a prefix
// cache already holds, and in a ceremony the open step's own writes.
func TestOnlyThePersonsNextRequestClosesATurnsWrites(t *testing.T) {
	content := strings.Repeat("línea de código\n", 200)
	write := recordCall(t, "w1", "local_write", `{"path":"a.go","content":`+quoteJSON(content)+`,"mode":"create"}`)
	exact := string(write.Arguments.Bytes())
	turn := []root.Message{
		{Role: root.RoleUser, Content: "escribe"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{write}},
		{Role: root.RoleTool, ToolCallID: "w1", Content: `{"status":"completed","output":{"written":true}}`},
	}
	answered := append(append([]root.Message(nil), turn...), root.Message{Role: root.RoleAssistant, Content: "hecho"})
	args := func(messages []root.Message) string {
		t.Helper()
		projection, err := NewDefaultModelContextProjector().Project(messages)
		if err != nil {
			t.Fatal(err)
		}
		return string(projection.Messages[1].ToolCalls[0].Arguments.Bytes())
	}
	for _, console := range []root.Text{memoryReminder, jevFinalPrefix + " ...", "[AXLR] The build step of ceremony axlr_delivery is still open."} {
		if got := args(append(append([]root.Message(nil), answered...), root.Message{Role: root.RoleUser, Content: console})); got != exact {
			t.Fatalf("console message %.20q closed the turn: %s", console, got)
		}
	}
	steered := append(append([]root.Message(nil), turn...), root.Message{Role: root.RoleUser, Content: "y también b.go"})
	if got := args(steered); got != exact {
		t.Fatalf("a steered message closed the turn: %s", got)
	}
	if got := args(append(append([]root.Message(nil), answered...), root.Message{Role: root.RoleUser, Content: "otra cosa"})); got == exact {
		t.Fatal("the person's next request did not close the turn")
	}
}

func quoteJSON(text string) string {
	encoded, _ := json.Marshal(text)
	return string(encoded)
}
