package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// bigTurn is one user turn whose tool results fit the per-result limit one
// by one but not the context budget together, like the MADE research turn
// that failed on 2 Oct 2026.
func bigTurn(t *testing.T, results, size int) []root.Message {
	t.Helper()
	args, _ := root.NewJSONObject([]byte(`{}`))
	messages := []root.Message{{Role: root.RoleUser, Content: "investiga"}}
	for i := 0; i < results; i++ {
		id := root.ToolCallID(fmt.Sprintf("c%d", i))
		messages = append(messages, root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: id, Name: "local_read", Arguments: args}}})
		body, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"text": strings.Repeat("x", size)}})
		messages = append(messages, root.Message{Role: root.RoleTool, ToolCallID: id, Content: root.Text(body)})
	}
	return messages
}

func TestATurnThatOutgrowsTheBudgetIsCompactedNotAborted(t *testing.T) {
	messages := bigTurn(t, scaled(12), 14000)
	projection, err := NewDefaultModelContextProjector().Project(messages)
	if err != nil {
		t.Fatalf("turn aborted: %v", err)
	}
	if !projection.TurnCompacted || projection.ProjectedBytes > domain.DefaultContextBudget().MaximumBytes() {
		t.Fatalf("compacted=%v bytes=%d", projection.TurnCompacted, projection.ProjectedBytes)
	}
	last := projection.Messages[len(projection.Messages)-1]
	if !strings.Contains(string(last.Content), "Answer with what you have") {
		t.Fatalf("the model is not told the turn was shortened: %.300s", last.Content)
	}
}

// A ceremony runs all its steps in one turn: one that wrote a few large
// files outgrows the prompt budget through the write arguments alone, which
// the step-down of tool results never reached. The compacted turn shortens
// the arguments of the writes that happened; the files hold their text.
func TestATurnThatWroteLargeFilesIsCompactedNotAborted(t *testing.T) {
	budget := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens)
	projector, err := NewModelContextProjector(budget)
	if err != nil {
		t.Fatal(err)
	}
	messages := []root.Message{{Role: root.RoleUser, Content: "write the generated tables"}}
	content := strings.Repeat("var table = []int{1, 2, 3, 4, 5, 6, 7, 8, 9}\n", 1200)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("w%d", i)
		messages = append(messages,
			root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{recordCall(t, id, "local_write", fmt.Sprintf(`{"path":"t%d.go","content":%s,"mode":"create"}`, i, quoteJSON(content)))}},
			root.Message{Role: root.RoleTool, ToolCallID: root.ToolCallID(id), Content: `{"status":"completed","output":{"written":true}}`})
	}
	if ModelMessagesBytes(messages) <= budget.MaximumBytes() {
		t.Fatal("the turn fits; the test proves nothing")
	}
	projection, err := projector.Project(messages)
	if err != nil {
		t.Fatalf("every continuation of this turn fails: %v", err)
	}
	if !projection.TurnCompacted || projection.ProjectedBytes > budget.MaximumBytes() {
		t.Fatalf("compacted=%v bytes=%d", projection.TurnCompacted, projection.ProjectedBytes)
	}
	var shortened map[string]any
	if err := json.Unmarshal(projection.Messages[1].ToolCalls[0].Arguments.Bytes(), &shortened); err != nil {
		t.Fatal(err)
	}
	recover, _ := shortened["recover"].(string)
	if shortened["path"] != "t0.go" || shortened["content_omitted_bytes"] != float64(len(content)) || !strings.Contains(recover, "local_read") || strings.Contains(recover, "closed") {
		t.Fatalf("current-turn write: %s", projection.Messages[1].ToolCalls[0].Arguments.Bytes())
	}
}

func TestAFittingTurnIsNotCompacted(t *testing.T) {
	projection, err := NewDefaultModelContextProjector().Project(bigTurn(t, 3, 2000))
	if err != nil || projection.TurnCompacted {
		t.Fatalf("compacted a turn that fits: %v %v", projection.TurnCompacted, err)
	}
}

func TestHistoryWillNotRereadAResultOfTheCurrentTurn(t *testing.T) {
	messages := append(bigTurn(t, 1, 100), bigTurn(t, 1, 100)...)
	args := func(i int) root.JSONValue {
		v, _ := root.NewJSONObject([]byte(fmt.Sprintf(`{"message_index":%d}`, i)))
		return v
	}
	if _, err := hostHistory(messages, args(5), MaxHistoryReadBytes); err == nil || !strings.Contains(err.Error(), "current turn") {
		t.Fatalf("current-turn result re-read: %v", err)
	}
	if _, err := hostHistory(messages, args(2), MaxHistoryReadBytes); err != nil {
		t.Fatalf("earlier turn refused: %v", err)
	}
	if _, err := hostHistory(messages, args(3), MaxHistoryReadBytes); err != nil {
		t.Fatalf("the current user message refused: %v", err)
	}
}
