package terminal

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ApprovalDialog owns display state only; the root handles its decision intent.
type ApprovalDialog struct {
	Pending domain.PendingTool
	Target  string
	Details Transcript
}

func NewApprovalDialog(p domain.PendingTool, target string) ApprovalDialog {
	var args bytes.Buffer
	_ = json.Indent(&args, p.Call.Arguments.Bytes(), "", "  ")
	d := ApprovalDialog{Pending: p, Target: target, Details: NewTranscript()}
	d.Details.SetContent(fmt.Sprintf("Tool: %s\nTarget: %s\nArguments:\n%s", p.Call.Name, target, args.String()))
	d.Details.Viewport.GotoTop()
	return d
}
func (d ApprovalDialog) Intent(msg tea.Msg) domain.ToolDecision {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "a":
			return domain.DecisionApprove
		case "d":
			return domain.DecisionDeny
		}
	}
	if click, ok := msg.(ControlIntent); ok {
		switch click {
		case "approve":
			return domain.DecisionApprove
		case "deny":
			return domain.DecisionDeny
		}
	}
	return ""
}
func (d ApprovalDialog) View(z *zone.Manager, prefix string) string {
	return "Tool approval — inspect arguments (↑↓ / PgUp PgDn)\n" + d.Details.View() + "\n" + z.Mark(prefix+"approve", "[Approve A]") + " " + z.Mark(prefix+"deny", "[Deny D]") + " " + z.Mark(prefix+"cancel", "[Cancel Esc]")
}
