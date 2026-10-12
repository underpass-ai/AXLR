package terminal

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// hiddenInputWarning names code a local exec runs that its program and
// arguments do not show at a glance: a stdin payload, which the card would
// otherwise leave below its fold, or inline code passed with -c, -e or -.
func (m AppModel) hiddenInputWarning(p domain.PendingTool) string {
	tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || tool.Identity.Kind != domain.ToolKindLocal || tool.Identity.LocalOperation != "exec" {
		return ""
	}
	var command struct {
		Args  []string `json:"args"`
		Stdin string   `json:"stdin"`
	}
	if json.Unmarshal(p.Call.Arguments.Bytes(), &command) != nil {
		return ""
	}
	icon := m.Theme.Icon("attention") + " "
	if command.Stdin != "" {
		lines := strings.Count(strings.TrimRight(command.Stdin, "\n"), "\n") + 1
		return icon + m.Theme.Tn("approval.stdinWarning", lines, lines)
	}
	if slices.ContainsFunc(command.Args, func(arg string) bool { return arg == "-c" || arg == "-e" || arg == "-" }) {
		return icon + m.Theme.T("approval.inlineCodeWarning")
	}
	return ""
}

// execCardCommand is a known local exec call's command line, for the top
// of its card; empty for any other call.
func (m AppModel) execCardCommand(p domain.PendingTool) string {
	tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || tool.Identity.Kind != domain.ToolKindLocal || tool.Identity.LocalOperation != "exec" {
		return ""
	}
	command, _ := execCommand(p.Call.Arguments.Bytes())
	return command
}

// modeAsksForEachCall is true when the session's mode, not the user's
// approval settings, keeps this call under approval; "always allow" would
// then only add a global rule the mode ignores until it leaves.
func (m AppModel) modeAsksForEachCall(p domain.PendingTool) bool {
	tool, args, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil {
		return false
	}
	mode := m.Header.State.Mode
	verdict, _ := mode.Judge(tool.Identity, args)
	return verdict == domain.VerdictAsk
}
