package application

import (
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
	"strings"
)

// modelMessages projects host capabilities without changing persisted conversation.
// Tool schemas and results remain complete; guide retrieval is progressive.
func modelMessages(s *domain.Session) []root.Message {
	var guidance strings.Builder
	guidance.WriteString("You are AXLR, an agent working in the user's local workspace. The supplied tools are real capabilities; use their schemas rather than guessing names. Respect user intent and tool errors. Tool approval is enforced by the host. Never claim a tool is unavailable when it is listed.\n")
	plugins := map[root.PluginID][]string{}
	for _, tool := range s.ToolSnapshot() {
		if tool.Identity.Kind == domain.ToolKindPlugin {
			id := tool.Identity.Plugin.PluginID
			plugins[id] = append(plugins[id], fmt.Sprintf("%s = %s", tool.Identity.Plugin.ToolName, tool.Definition.Name))
		}
	}
	ids := make([]string, 0, len(plugins))
	for id := range plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, raw := range ids {
		id := root.PluginID(raw)
		switch id {
		case "kmp":
			guidance.WriteString("KMP is Underpass graph-temporal agent memory. It recovers stored evidence and records decisions, constraints and outcomes. Recover relevant project context before re-deriving it; UNKNOWN is a valid answer. Read only the brief entry of kmp_guide initially, and request a specific extended topic only when needed for the current operation. Reuse guidance already present in this conversation; do not fetch all guide topics or reread them every turn. Use explicit project scope and evidence.\n")
		case "made":
			guidance.WriteString("MADE is Underpass's engine for agentic ceremonies: structured procedures, working sessions, review loops and human approval. It is available through the registered MADE MCP tools. Discover existing ceremonies and their required transitions through its tools; never invent ceremony results or approvals.\n")
		}
		fmt.Fprintf(&guidance, "Registered MCP plugin %s tool names: %s.\n", id, strings.Join(plugins[id], ", "))
	}
	return append([]root.Message{{Role: root.RoleSystem, Content: root.Text(guidance.String())}}, s.Messages()...)
}
