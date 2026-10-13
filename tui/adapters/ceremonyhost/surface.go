package ceremonyhost

import (
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The engines' protocols as the console owns them (decisions D3, D6 and
// G6 of the lifecycle integration design): what the model may call of
// KMP and MADE, which argument carries the session's KMP context, and
// which call is the memory write. The application reads the result from
// each tool's surface and never names an engine.

// Plugin ids of the two engines the console runs.
const (
	memoryPluginID        root.PluginID = "kmp"
	orchestratorPluginID  root.PluginID = "made"
	memoryWriteTool                     = "kmp_write_memory"
	memoryGuideTool                     = "kmp_guide"
	memoryContextArgument               = "context_id"
)

// memoryOwnedRefusal answers a model that reaches for KMP's identity:
// registering an agent, opening a context and choosing the store are the
// console's alone, and it says what to do instead, since the guidance of
// older sessions still names kmp_guide.
const memoryOwnedRefusal = "kmp_guide is the console's: it registered this session with KMP and passes the session's context on every KMP call; ask memory with axlr_recall, or call kmp_ask, kmp_wake and kmp_write_memory through axlr_call_tool without context_id"

// orchestratorOwnedRefusal answers a model that reaches for MADE's write
// surface: the console starts, claims, completes and transitions every
// ceremony, this session's included.
const orchestratorOwnedRefusal = "the console runs ceremonies: it starts, claims, completes, transitions, publishes and cancels them itself, this session's included; read definitions and instances with the made_get_*, made_list_*, made_read_*, made_search_* and made_inspect_* verbs, design and validate drafts, and ask the person to run or publish one"

// madeReadVerbs are the MADE verbs the model may call: reads, searches,
// streams and design-time checks; nothing that starts, claims, completes,
// transitions, cancels, pauses, resumes, runs, approves, defers, grants,
// revokes, publishes, registers, deletes or uploads.
var (
	madeReadPrefixes = []string{"list_", "get_", "read_", "stream_", "search_", "inspect_"}
	madeReadVerbs    = map[string]bool{"get_help": true, "discover_capabilities": true, "design_ceremony": true, "validate_ceremony_draft": true, "explain_ceremony_draft": true, "diff_ceremony_definitions": true, "render_agentic_system_diagram": true, "generate_ceremony_report": true, "verify_ceremony_journal": true}
)

// Surface is what the console owns of one plugin tool; a tool of any
// other plugin is the model's to call, with nothing filled in.
func Surface(ref root.PluginRef) domain.ToolSurface {
	name := ref.ToolName.String()
	switch ref.PluginID {
	case memoryPluginID:
		surface := domain.ToolSurface{Role: domain.PluginRoleMemory, ContextArgument: memoryContextArgument, MemoryWrite: name == memoryWriteTool, Entry: name == memoryWriteTool || strings.Contains(name, "guide")}
		if name == memoryGuideTool {
			surface.Hidden, surface.ContextArgument = memoryOwnedRefusal, ""
		}
		return surface
	case orchestratorPluginID:
		surface := domain.ToolSurface{Role: domain.PluginRoleOrchestrator, Entry: strings.Contains(name, "guide")}
		if !madeRead(strings.TrimPrefix(name, "made_")) {
			surface.Hidden = orchestratorOwnedRefusal
		}
		return surface
	}
	return domain.ToolSurface{Entry: strings.Contains(name, "guide")}
}

func madeRead(verb string) bool {
	if madeReadVerbs[verb] {
		return true
	}
	for _, prefix := range madeReadPrefixes {
		if strings.HasPrefix(verb, prefix) {
			return true
		}
	}
	return false
}
