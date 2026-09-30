package terminal

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ApprovalDialog owns display state only; the root handles its decision intent.
type ApprovalDialog struct {
	Pending domain.PendingTool
	Target  string
	Details Transcript
}

func NewApprovalDialog(p domain.PendingTool, target string, locales ...Locale) ApprovalDialog {
	locale := English
	if len(locales) > 0 {
		locale = locales[0]
	}
	var args bytes.Buffer
	_ = json.Indent(&args, p.Call.Arguments.Bytes(), "", "  ")
	d := ApprovalDialog{Pending: p, Target: target, Details: NewTranscript()}
	d.Details.SetContent(Translatef(locale, "approval.details", p.Call.Name, target, args.String()))
	d.Details.Viewport.GotoTop()
	return d
}
func (d ApprovalDialog) Intent(msg tea.Msg) domain.ToolDecision {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "a":
			return domain.DecisionApprove
		case "l":
			return domain.DecisionAutoApprove
		case "d":
			return domain.DecisionDeny
		}
	}
	if click, ok := msg.(ControlIntent); ok {
		switch click {
		case "approve":
			return domain.DecisionApprove
		case "always-allow":
			return domain.DecisionAutoApprove
		case "deny":
			return domain.DecisionDeny
		}
	}
	return ""
}
func (d ApprovalDialog) View(theme Theme, z *zone.Manager, prefix string, width, height int) string {
	footer := z.Mark(prefix+"approve", "["+theme.T("approval.approve")+"]") + "  " + z.Mark(prefix+"always-allow", "["+theme.T("approval.alwaysAllow")+"]") + "  " + z.Mark(prefix+"deny", "["+theme.T("approval.deny")+"]") + "  " + z.Mark(prefix+"cancel", "["+theme.T("common.cancel")+"]")
	return theme.Overlay(theme.T("approval.title"), theme.T("approval.subtitle"), d.Details.View(), footer, width, height)
}
