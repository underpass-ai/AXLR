package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
)

const (
	HostToolsName    root.ToolName = "axlr_tools"
	HostCallToolName root.ToolName = "axlr_call_tool"
	HostHistoryName  root.ToolName = "axlr_history"
	HostSkillName    root.ToolName = "axlr_skill"
	HostSessionName  root.ToolName = "axlr_session"
	HostStepDoneName root.ToolName = "axlr_step_done"
	// HostRequestRepairName asks the console to repair AXLR itself in a
	// separate session; HostRepairStatusName reads the linked repairs.
	HostRequestRepairName root.ToolName = "axlr_request_repair"
	HostRepairStatusName  root.ToolName = "axlr_repair_status"
	// HostJudgeName asks TypeSafe Jev one question; see JudgeTool.
	HostJudgeName       root.ToolName = "axlr_judge"
	MaxHostResultBytes                = 64 * 1024
	MaxHistoryReadBytes               = 32 * 1024
)

// HostTools is the small, stable model surface for discovering and calling
// registered plugin tools and recovering messages outside the request context.
func HostTools() []domain.AvailableTool {
	specs := []struct {
		name                           root.ToolName
		operation, description, schema string
	}{
		{HostSessionName, domain.HostOperationSession, "Read the current session's ID, workspace, user prompt count, title and exact KMP about. Optional title/about fill only missing fields and preserve user titles and archive flags. Set a concise title after the second user prompt. This is session bookkeeping; it does not write KMP memory.", `{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":120},"about":{"type":"string","minLength":1,"maxLength":256}},"additionalProperties":false}`},
		{HostCallToolName, domain.HostOperationCallTool, "Call a registered plugin tool by the exact name returned by axlr_tools. The real plugin's approval policy applies. Local and host tools cannot be called through this bridge.", `{"type":"object","properties":{"name":{"type":"string","minLength":1},"arguments":{"type":"object"}},"required":["name","arguments"],"additionalProperties":false}`},
		{HostHistoryName, domain.HostOperationHistory, "Recover a persisted session message. message_index is zero-based; offset_bytes and limit_bytes page the UTF-8 bytes of its content (text), and the first page also lists any tool calls. Use next_offset_bytes to continue. Maximum page is 32768 bytes.", `{"type":"object","properties":{"message_index":{"type":"integer","minimum":0},"offset_bytes":{"type":"integer","minimum":0},"limit_bytes":{"type":"integer","minimum":1,"maximum":32768}},"required":["message_index"],"additionalProperties":false}`},
		{HostToolsName, domain.HostOperationTools, "Search registered plugin tools with query, or get one exact tool's schema with name (x-* annotations omitted). A schema too large to return comes back as an outline; repeat with path, a JSON pointer such as /properties/intent, to get that part exactly. An empty query lists compact summaries; limit defaults to 8, maximum 20. Use offset with next_offset for more matches. Discovery has no plugin effects.", `{"type":"object","properties":{"query":{"type":"string"},"name":{"type":"string","minLength":1},"path":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":20},"offset":{"type":"integer","minimum":0}},"additionalProperties":false}`},
		{HostStepDoneName, domain.HostOperationStepDone, "Hand the current ceremony step's result to the console, which runs its checks, records the step in MADE and replies with the next step. Send only the fields the current step asks for. check_command is a program and its arguments, run without a shell; a new command needs the user's approval once. In a repair, connect_to links the diagnosed cause to memory refs the recall exposed: objects with ref, rel and why.", `{"type":"object","properties":{"check_command":{"type":"object","properties":{"program":{"type":"string","minLength":1},"args":{"type":"array","items":{"type":"string"}}},"required":["program"],"additionalProperties":false},"reproducible":{"type":"boolean"},"expected":{"type":"string"},"observed":{"type":"string"},"root_cause":{"type":"string"},"evidence":{"type":"string"},"proposed_fix":{"type":"string"},"criteria":{"type":"string"},"scope":{"type":"string"},"summary":{"type":"string"},"report":{"type":"string"},"summary_en":{"type":"string"},"connect_to":{"type":"array","maxItems":8,"items":{"type":"object","properties":{"ref":{"type":"string","minLength":1},"rel":{"type":"string","minLength":1},"why":{"type":"string","minLength":1},"evidence":{"type":"string"}},"required":["ref","rel","why"],"additionalProperties":false}},"impact":{"type":"string"},"severity":{"type":"string","enum":["sev1","sev2","sev3","sev4"]},"detection":{"type":"string"},"service":{"type":"string"},"slug":{"type":"string"},"timeline":{"type":"array","items":{"type":"object","properties":{"at":{"type":"string"},"event":{"type":"string"},"evidence":{"type":"string"}},"required":["at","event","evidence"],"additionalProperties":false}},"contributing_factors":{"type":"array","items":{"type":"string"}},"went_well":{"type":"array","items":{"type":"string"}},"went_badly":{"type":"array","items":{"type":"string"}},"actions":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string"},"kind":{"type":"string","enum":["corrective","preventive"]},"owner":{"type":"string"},"due":{"type":"string"},"verification":{"type":"string"}},"required":["title","kind","owner","due","verification"],"additionalProperties":false}},"draft_path":{"type":"string"},"tasks":{"type":"array","minItems":1,"maxItems":12,"items":{"type":"object","properties":{"id":{"type":"string"},"goal":{"type":"string"},"scope":{"type":"array","items":{"type":"string"}},"context":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"line":{"type":"integer"},"quote":{"type":"string"}},"required":["path","line","quote"],"additionalProperties":false}},"unit_check":{"type":"object","properties":{"program":{"type":"string","minLength":1},"args":{"type":"array","items":{"type":"string"}}},"required":["program"],"additionalProperties":false},"depends_on":{"type":"array","items":{"type":"string"}},"test_first":{"type":"boolean"},"protect":{"type":"array","items":{"type":"string"}}},"required":["id","goal","scope","unit_check"],"additionalProperties":false}},"e2e_check":{"type":"object","properties":{"program":{"type":"string","minLength":1},"args":{"type":"array","items":{"type":"string"}}},"required":["program"],"additionalProperties":false},"interfaces":{"type":"string"},"test_files":{"type":"array","items":{"type":"string"}},"untestable":{"type":"boolean"},"notes":{"type":"array","items":{"type":"object","properties":{"to":{"type":"string"},"text":{"type":"string"}},"required":["to","text"],"additionalProperties":false}},"questions":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`},
		{HostRequestRepairName, domain.HostOperationRequestRepair, "Ask the console to repair a defect of AXLR itself (its local_* and axlr_* tools, its runtime) in a separate repair session, without stopping this one, under the self-repair rule of the system prompt. Use it only after the same AXLR operation failed the same way twice in this session; cite those tool_calls by ID. The console refuses other failures with the reason, keeps the reproduction command and the merge under the person's approval and reports the outcome to this session; a merged fix does not change this running console.", `{"type":"object","properties":{"description":{"type":"string","minLength":20,"maxLength":2000},"expected":{"type":"string","minLength":1,"maxLength":2000},"observed":{"type":"string","minLength":1,"maxLength":2000},"evidence":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","minLength":1,"maxLength":1000}},"tool_calls":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","minLength":1}}},"required":["description","expected","observed","evidence","tool_calls"],"additionalProperties":false}`},
		{HostRepairStatusName, domain.HostOperationRepairStatus, "Read the self-repairs linked to this session: status, step, pull request, pending decision, outcome and the KMP/MADE reports. Optional repair selects one by ID. Read-only.", `{"type":"object","properties":{"repair":{"type":"string","minLength":1}},"additionalProperties":false}`},
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

// JudgeTool is axlr_judge, added to a session's host tools only when the
// console enables Jev, so the default prefix does not grow.
func JudgeTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationJudge)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"state":{"type":"string","minLength":1,"maxLength":24576},"question":{"type":"string","minLength":1,"maxLength":2000},"options":{"type":"array","minItems":2,"maxItems":16,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":200}}},"required":["state","question"],"additionalProperties":false}`))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostJudgeName,
		Description: "Ask Jev, an independent judgement model, one question at a real fork in your task: which approach or next step to take, or whether a result satisfies the task. Put in state everything it needs (the task, what you found, the candidates and their evidence): Jev does not see this conversation or the files and does nothing. A question between alternatives (which one, A or B) must list them in options, and Jev returns the choice, a probability per option and its confidence; without options the question must be answerable yes or no, and Jev returns only the probability of yes. You still decide. Do not ask it facts your tools can check.",
		Parameters:  schema,
	}}
}

// ModelTools projects authority into the fixed local and host tool surface.
// Plugin schemas remain in the frozen snapshot for discovery and authorization.
func ModelTools(snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := make([]root.ToolDefinition, 0, 9)
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
// while a ceremony runs and its step is the model's, not the person's, and
// axlr_request_repair never reaches a repair session.
func SessionTools(s domain.Session, snapshot []domain.AvailableTool) []root.ToolDefinition {
	tools := ModeTools(s.Mode(), snapshot)
	if _, step, focused := focusedRun(s); focused {
		return compactTools(tools, step)
	}
	run, live := s.Ceremony()
	stepDone := live && !run.AwaitingPerson()
	out := tools[:0:0]
	for _, tool := range tools {
		switch {
		case tool.Name == HostStepDoneName && !stepDone:
		case tool.Name == HostRequestRepairName && s.Mode() == domain.ModeRepair:
		default:
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
