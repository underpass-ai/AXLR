package ceremonyhost

import (
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The engines' protocols as the console owns them (decisions D6 and G6
// of the lifecycle integration design): what the model may call of KMP
// and MADE, and which call is the memory write. The application reads the
// result from each tool's surface and never names an engine. KMP 1.0 has
// no agents or contexts: its kmp_guide is a read-only topic lookup the
// model may call like any other read.

// Plugin ids of the two engines the console runs.
const (
	memoryPluginID       root.PluginID = "kmp"
	orchestratorPluginID root.PluginID = "made"
	memoryWriteTool                    = "kmp_write_memory"
)

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
		return domain.ToolSurface{Role: domain.PluginRoleMemory, MemoryWrite: name == memoryWriteTool}
	case orchestratorPluginID:
		surface := domain.ToolSurface{Role: domain.PluginRoleOrchestrator}
		if !madeRead(strings.TrimPrefix(name, "made_")) {
			surface.Hidden = orchestratorOwnedRefusal
		}
		return surface
	}
	return domain.ToolSurface{}
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
