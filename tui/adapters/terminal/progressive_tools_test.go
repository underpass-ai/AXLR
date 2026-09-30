package terminal

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestWrappedMemoryRowsAndApprovalShowActualPlugin(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_ask"})
	args, _ := root.NewJSONObject([]byte(`{"name":"memory","arguments":{"about":"project:fixture"}}`))
	call := root.ToolCall{ID: "memory", Name: application.HostCallToolName, Arguments: args}
	state := m.Header.State
	state.ToolSnapshot = append(application.HostTools(), domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{Name: "memory"}})
	state.Status = domain.StatusApproval
	state.Activity = []domain.PendingTool{{Call: call}}
	state.Messages = []root.Message{{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}}
	m.Header.State = state
	m.syncApproval()
	if m.Approval.Target != "plugin kmp / kmp_ask" {
		t.Fatal("generic host wrapper hid the effect target", m.Approval.Target)
	}
	m.Transcript.SetSession(state, "", m.Theme)
	if !strings.Contains(m.Transcript.Viewport.GetContent(), "memory request: kmp / kmp_ask") {
		t.Fatal("wrapped memory did not retain memory styling")
	}
}
