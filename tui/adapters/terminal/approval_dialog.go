package terminal

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
)

// ApprovalDialog owns display state only; the root handles its decision intent.
type ApprovalDialog struct {
	Pending domain.PendingTool
	Target  string
	Details Transcript
}

// NewApprovalDialog's details are the call's arguments; the card and the
// unknown-tool dialog show the tool and its target in their own title. A
// file edit or write is laid out as the change it makes (fileChangeRows).
func NewApprovalDialog(p domain.PendingTool, target string, theme Theme) ApprovalDialog {
	var args bytes.Buffer
	_ = json.Indent(&args, p.Call.Arguments.Bytes(), "", "  ")
	content := args.String()
	if p.Call.Name == application.HostForgeToolName {
		if readable, ok := forgeCard(p.Call.Arguments.Bytes()); ok {
			content = readable
		}
	}
	d := ApprovalDialog{Pending: p, Target: target, Details: NewTranscript()}
	d.Details.theme = theme
	if rows, ok := fileChangeRows(p.Call.Name, p.Call.Arguments.Bytes(), theme); ok {
		d.Details.setRows(rows)
	} else {
		d.Details.SetContent(content)
	}
	d.Details.Viewport.GotoTop()
	return d
}
func (d ApprovalDialog) Intent(msg tea.Msg) domain.ToolDecision {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		// The help prints A, L and D: Shift+letter is the same key.
		switch strings.ToLower(k.String()) {
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
