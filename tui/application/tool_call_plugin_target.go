package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// CallPluginTool is the plugin's tool a call reaches, as plugin/tool, by
// its own name, its opaque alias or through axlr_call_tool, resolved
// against the session's snapshot as the run resolves it; empty for the
// console's own tools and for a call that does not resolve.
func CallPluginTool(snapshot []domain.AvailableTool, call root.ToolCall) string {
	target, _, known, err := ResolveToolCall(snapshot, call)
	if err != nil || !known || target.Identity.Kind != domain.ToolKindPlugin {
		return ""
	}
	return target.Identity.Plugin.PluginID.String() + "/" + target.Identity.Plugin.ToolName.String()
}
