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
	if shortened["path"] != "a.go" || shortened["mode"] != "create" || shortened["content"] != nil || shortened["content_omitted_bytes"] != float64(len(content)) || !strings.Contains(shortened["recover"].(string), "local_read") {
		t.Fatalf("closed write: %s", calls[0].Arguments.Bytes())
	}
	if err := json.Unmarshal(calls[1].Arguments.Bytes(), &shortened); err != nil {
		t.Fatal(err)
	}
	if shortened["path"] != "b.go" || shortened["old_text_omitted_bytes"] != float64(len(content)) || shortened["new_text_omitted_bytes"] != float64(1) {
		t.Fatalf("closed edit: %s", calls[1].Arguments.Bytes())
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
	results := []root.Text{"tool call denied by user", `{"protocol_version":1,"tool":"write","status":"failed","error":{"code":"io_error","message":"disk full"}}`}
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
		if got := string(projection.Messages[1].ToolCalls[0].Arguments.Bytes()); got != string(write.Arguments.Bytes()) {
			t.Fatalf("%.30q: a write that did not happen was shortened to %s", result, got)
		}
	}
}

func quoteJSON(text string) string {
	encoded, _ := json.Marshal(text)
	return string(encoded)
}
