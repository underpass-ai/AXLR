package application

import (
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// engineSurface stands in for the engine adapter's protocol table
// (ceremonyhost.Surface): the fixtures' kmp and made tools get the
// surface the console runs them with.
func engineSurface(ref root.PluginRef) domain.ToolSurface {
	name := string(ref.ToolName)
	switch ref.PluginID {
	case "kmp":
		if name == "kmp_guide" {
			return domain.ToolSurface{Role: domain.PluginRoleMemory, Hidden: "kmp_guide is the console's"}
		}
		return domain.ToolSurface{Role: domain.PluginRoleMemory, ContextArgument: "context_id", MemoryWrite: name == "kmp_write_memory"}
	case "made":
		verb := strings.TrimPrefix(name, "made_")
		for _, prefix := range []string{"list_", "get_", "read_", "stream_", "search_", "inspect_", "design_", "validate_"} {
			if strings.HasPrefix(verb, prefix) {
				return domain.ToolSurface{Role: domain.PluginRoleOrchestrator}
			}
		}
		return domain.ToolSurface{Role: domain.PluginRoleOrchestrator, Hidden: "the console runs ceremonies"}
	}
	return domain.ToolSurface{}
}

// engineParagraphs stands in for the engine adapter's orientation
// (ceremonyhost.Guidance), with the record sentence the facts call for.
type engineParagraphs struct{}

func (engineParagraphs) Paragraph(plugin root.PluginID, facts PluginGuidanceFacts) string {
	switch plugin {
	case "kmp":
		text := "Memory (KMP): the console recalls and records.\n"
		switch {
		case facts.RecordsMemory && facts.Remembers:
			text += "Record what the work settles: it goes to axlr_remember.\n"
		case facts.RecordsMemory:
			text += "Record what the work settles: it goes to kmp_write_memory through axlr_call_tool.\n"
		}
		return text
	case "made":
		return "Ceremonies (MADE): the console runs them.\n"
	}
	return ""
}

// memoryWriteSnapshot is a snapshot whose memory write is kmp_write_memory.
func memoryWriteSnapshot() []domain.AvailableTool {
	ref := root.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"}
	identity, _ := domain.NewPluginToolIdentity(ref)
	return []domain.AvailableTool{{Identity: identity, Definition: root.ToolDefinition{Name: "kmp_write_memory"}, Surface: engineSurface(ref)}}
}
