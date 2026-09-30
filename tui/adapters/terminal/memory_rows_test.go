package terminal

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func memoryState() domain.SessionState {
	args, _ := root.NewJSONObject([]byte(`{"about":"project:AXLR"}`))
	return domain.SessionState{ToolSnapshot: []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "mcp_alias"}, Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: "kmp", ToolName: "kmp_wake"}}}}, Messages: []root.Message{{Role: root.RoleUser, Content: "remember"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call", Name: "mcp_alias", Arguments: args}}}, {Role: root.RoleTool, ToolCallID: "call", Content: root.Text(strings.Repeat("memory evidence ", 10000))}, {Role: root.RoleAssistant, Content: "recovered"}}, Activity: []domain.PendingTool{{Call: root.ToolCall{ID: "call", Name: "mcp_alias", Arguments: args}, Decision: domain.DecisionApprove}}}
}
func TestMemoryRowsAreCompactDistinctAndChronological(t *testing.T) {
	state := memoryState()
	tr := NewTranscript()
	tr.Viewport.SetWidth(100)
	tr.Viewport.SetHeight(30)
	tr.SetSession(state, "", Theme{})
	text := tr.Viewport.GetContent()
	if len(text) > 1000 || !strings.Contains(text, "memory request: kmp / kmp_wake") || !strings.Contains(text, "full result saved") || strings.Contains(text, "mcp_alias") {
		t.Fatal("tool rows are not bounded and identifiable")
	}
	if strings.Index(text, "user: remember") >= strings.Index(text, "memory request:") || strings.Index(text, "memory result:") >= strings.Index(text, "assistant: recovered") {
		t.Fatal("conversation order is wrong")
	}
	if !strings.Contains(tr.View(), "48;2;48;40;61") {
		t.Fatal("memory row has no distinct background")
	}
	if state.Messages[2].Content != root.Text(strings.Repeat("memory evidence ", 10000)) {
		t.Fatal("rendering changed saved memory evidence")
	}
}
func TestSnapshotUpdatesDuringAutomaticToolsClearCompletedDraft(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.draft = "previous answer"
	m.draftOperationID = 1
	m.operationID = 1
	state := memoryState()
	m = update(m, application.Event{Kind: application.EventSession, Snapshot: &state})
	if m.draft != "" || !strings.Contains(m.Transcript.Viewport.GetContent(), "memory result:") {
		t.Fatal("persisted snapshot not visible between automatic calls")
	}
	m = update(m, application.Event{Kind: application.EventToolExecutionStarted, Memory: true})
	if !strings.Contains(m.View().Content, "Memory · KMP is running") {
		t.Fatal("active memory status absent")
	}
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if strings.Contains(m.View().Content, "Memory · KMP is running") {
		t.Fatal("memory status stale during model response")
	}
}
func TestFullResultsRemainAvailableInInfoDetail(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Header.State = memoryState()
	m.Header.State.Activity[0].Outcome = &domain.ToolOutcome{Content: "full precise memory evidence"}
	m = update(m, ControlIntent("info"))
	if !strings.Contains(m.Info.Viewport.GetContent(), "full precise memory evidence") {
		t.Fatal("tool detail lost")
	}
}

func TestHistoricalHashedToolsKeepMemoryPresentationWithNativeSnapshot(t *testing.T) {
	const oldAlias root.ToolName = "mcp_7e2b4391433acc541013b46c061f86ec7da1de3d87ae6fbf"
	state := memoryState()
	state.ToolSnapshot[0].Definition.Name = "kmp_wake"
	state.Messages[1].ToolCalls[0].Name = oldAlias
	state.Activity[0].Call.Name = oldAlias
	state.Activity[0].Outcome = &domain.ToolOutcome{Content: "historical memory evidence"}
	tr := NewTranscript()
	tr.Viewport.SetWidth(100)
	tr.Viewport.SetHeight(30)
	tr.SetSession(state, "", Theme{})
	text := tr.Viewport.GetContent()
	if strings.Contains(text, string(oldAlias)) || !strings.Contains(text, "memory request: kmp / kmp_wake") || !strings.Contains(text, "memory result: kmp / kmp_wake") {
		t.Fatal("historical tools lost their identity when snapshot switched to native names")
	}
	if !strings.Contains(tr.View(), "48;2;48;40;61") {
		t.Fatal("historical memory rows lost their background")
	}
	m := sized()
	defer m.zones.Close()
	m.Header.State = state
	m = update(m, ControlIntent("info"))
	detail := m.Info.Viewport.GetContent()
	if !strings.Contains(detail, "kmp / kmp_wake") || !strings.Contains(detail, "historical memory evidence") || strings.Contains(detail, string(oldAlias)) {
		t.Fatal("historical tool details lost human labels or full outcome")
	}
	// A guessed alias for another plugin has no match or memory presentation.
	label, memory := toolPresentation(state, "mcp_unmatched")
	if label != "mcp_unmatched" || memory {
		t.Fatal("unmatched historical alias gained memory identity")
	}
}
