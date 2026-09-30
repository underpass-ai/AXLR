package terminal

import (
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func approvalContent(settings application.ApprovalSettingsPort, locale Locale) string {
	if settings == nil {
		return Translate(locale, "error.approvalSettings")
	}
	state := Translate(locale, "approvals.off")
	if settings.Autonomous() {
		state = Translate(locale, "approvals.on")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n\n%s", Translate(locale, "approvals.mode"), state, Translate(locale, "approvals.list"))
	allowed := settings.Allowed()
	if len(allowed) == 0 {
		b.WriteString("\n" + Translate(locale, "approvals.empty"))
	}
	for _, id := range allowed {
		if id.Kind == domain.ToolKindPlugin {
			fmt.Fprintf(&b, "\n• %s / %s", id.Plugin.PluginID, id.Plugin.ToolName)
		} else {
			fmt.Fprintf(&b, "\n• %s", id.LocalOperation)
		}
	}
	return b.String()
}
