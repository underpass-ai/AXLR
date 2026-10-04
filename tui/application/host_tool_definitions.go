package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
)

const (
	HostToolsName       root.ToolName = "axlr_tools"
	HostCallToolName    root.ToolName = "axlr_call_tool"
	HostHistoryName     root.ToolName = "axlr_history"
	HostSkillName       root.ToolName = "axlr_skill"
	HostStepDoneName    root.ToolName = "axlr_step_done"
	MaxHostResultBytes                = 32 * 1024
	MaxHistoryReadBytes               = 16 * 1024
)

// HostTools is the small, stable model surface for discovering and calling
// registered plugin tools and recovering messages outside the request context.
func HostTools() []domain.AvailableTool {
	specs := []struct {
		name                           root.ToolName
		operation, description, schema string
	}{
		{HostCallToolName, domain.HostOperationCallTool, "Call a registered plugin tool by the exact name returned by axlr_tools. The real plugin's approval policy applies. Local and host tools cannot be called through this bridge.", `{"type":"object","properties":{"name":{"type":"string","minLength":1},"arguments":{"type":"object"}},"required":["name","arguments"],"additionalProperties":false}`},
		{HostHistoryName, domain.HostOperationHistory, "Recover a full persisted session message as paged JSON. message_index is zero-based; offset_bytes and limit_bytes page its UTF-8 JSON bytes. Use next_offset_bytes to continue. Maximum page is 16384 bytes.", `{"type":"object","properties":{"message_index":{"type":"integer","minimum":0},"offset_bytes":{"type":"integer","minimum":0},"limit_bytes":{"type":"integer","minimum":1,"maximum":16384}},"required":["message_index"],"additionalProperties":false}`},
		{HostToolsName, domain.HostOperationTools, "Search registered plugin tools with query, or get one exact tool's full schema with name. An empty query lists compact summaries; limit defaults to 8, maximum 20. Use offset with next_offset for more matches. Discovery has no plugin effects.", `{"type":"object","properties":{"query":{"type":"string"},"name":{"type":"string","minLength":1},"limit":{"type":"integer","minimum":1,"maximum":20},"offset":{"type":"integer","minimum":0}},"additionalProperties":false}`},
		{HostStepDoneName, domain.HostOperationStepDone, "Hand the current ceremony step's result to the console, which runs its checks, records the step in MADE and replies with the next step. Send only the fields the current step asks for. check_command is a program and its arguments, run without a shell; a new command needs the user's approval once.", `{"type":"object","properties":{"check_command":{"type":"object","properties":{"program":{"type":"string","minLength":1},"args":{"type":"array","items":{"type":"string"}}},"required":["program"],"additionalProperties":false},"reproducible":{"type":"boolean"},"expected":{"type":"string"},"observed":{"type":"string"},"root_cause":{"type":"string"},"evidence":{"type":"string"},"proposed_fix":{"type":"string"},"criteria":{"type":"string"},"scope":{"type":"string"},"summary":{"type":"string"},"report":{"type":"string"},"summary_en":{"type":"string"},"impact":{"type":"string"},"severity":{"type":"string","enum":["sev1","sev2","sev3","sev4"]},"detection":{"type":"string"},"service":{"type":"string"},"slug":{"type":"string"},"timeline":{"type":"array","items":{"type":"object","properties":{"at":{"type":"string"},"event":{"type":"string"},"evidence":{"type":"string"}},"required":["at","event","evidence"],"additionalProperties":false}},"contributing_factors":{"type":"array","items":{"type":"string"}},"went_well":{"type":"array","items":{"type":"string"}},"went_badly":{"type":"array","items":{"type":"string"}},"actions":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string"},"kind":{"type":"string","enum":["corrective","preventive"]},"owner":{"type":"string"},"due":{"type":"string"},"verification":{"type":"string"}},"required":["title","kind","owner","due","verification"],"additionalProperties":false}},"draft_path":{"type":"string"}},"additionalProperties":false}`},
		{HostSkillName, domain.HostOperationSkill, "Read an installed AXLR plugin skill or its text references. Use plugin and skill names from the installed skill index. path defaults to SKILL.md and is relative to that skill directory; paths such as ../../references/example.md are supported within the installed package. Use next_offset_bytes for more pages.", `{"type":"object","properties":{"plugin":{"type":"string","minLength":1},"skill":{"type":"string","minLength":1},"path":{"type":"string","minLength":1},"offset_bytes":{"type":"integer","minimum":0},"limit_bytes":{"type":"integer","minimum":1,"maximum":4096}},"required":["plugin","skill"],"additionalProperties":false}`},
	}
	tools := make([]domain.AvailableTool, 0, len(specs))
	for _, spec := range specs {
		identity, _ := domain.NewHostToolIdentity(spec.operation)
		schema, _ := root.NewJSONObject([]byte(spec.schema))
		tools = append(tools, domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{Name: spec.name, Description: root.Text(spec.description), Parameters: schema}})
	}
	return tools
}

// ModelTools projects authority into the fixed local and host tool surface.
// Plugin schemas remain in the frozen snapshot for discovery and authorization.
func ModelTools(snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := make([]root.ToolDefinition, 0, 7)
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindLocal || tool.Identity.Kind == domain.ToolKindHost {
			tools = append(tools, tool.Definition)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

// ModeTools is the model's tool surface under a work mode. Hidden tools are
// also refused by the mode if a model calls them from memory.
// SessionTools is ModeTools for a session: axlr_step_done is offered only
// while a ceremony runs and its step is the model's, not the person's.
func SessionTools(s domain.Session, snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := ModeTools(s.Mode(), snapshot)
	if run, live := s.Ceremony(); live && !run.AwaitingPerson() {
		return tools
	}
	out := tools[:0:0]
	for _, tool := range tools {
		if tool.Name != HostStepDoneName {
			out = append(out, tool)
		}
	}
	return out
}

func ModeTools(mode domain.WorkMode, snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := ModelTools(snapshot)
	if !mode.HidesWriteTools() {
		return tools
	}
	out := tools[:0:0]
	for _, tool := range tools {
		if tool.Name != "local_write" && tool.Name != "local_edit" {
			out = append(out, tool)
		}
	}
	return out
}
