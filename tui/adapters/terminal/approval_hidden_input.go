package terminal

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

// removalWarning warns that a local exec deletes, moves or truncates files,
// which /undo cannot put back; a command a destructive rule already names
// says so in the card's title instead.
func (m AppModel) removalWarning(p domain.PendingTool) string {
	tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || tool.Identity.Kind != domain.ToolKindLocal || tool.Identity.LocalOperation != "exec" {
		return ""
	}
	var command struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}
	if json.Unmarshal(p.Call.Arguments.Bytes(), &command) != nil || !domain.RemovesFiles(command.Program, command.Args) || m.destructiveRule(p) != "" {
		return ""
	}
	return m.Theme.Icon("attention") + " " + m.Theme.T("approval.removesFiles")
}

// repeatedProgramWarning warns that a local exec's first argument repeats
// its program: the model sent {"program":"echo","args":["echo","hola"]},
// which runs "echo echo hola", on 14 October 2026.
func (m AppModel) repeatedProgramWarning(p domain.PendingTool) string {
	tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || tool.Identity.Kind != domain.ToolKindLocal || tool.Identity.LocalOperation != "exec" {
		return ""
	}
	var command struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}
	if json.Unmarshal(p.Call.Arguments.Bytes(), &command) != nil || len(command.Args) == 0 || strings.TrimSpace(command.Program) == "" {
		return ""
	}
	if first := command.Args[0]; first != command.Program && first != filepath.Base(command.Program) {
		return ""
	}
	return m.Theme.Icon("attention") + " " + m.Theme.Tf("approval.repeatsProgram", singleLine(command.Program))
}

// execCardCommand is a known local exec call's card: the command line as a
// shell would read it, then the working directory, timeout, output limit
// and stdin only when the call sets them. On 12 October 2026 an exec card
// showed "$ go vet ./buildinfo/..." and then the same arguments again as
// JSON. A call with fields local_exec does not have keeps its JSON below.
// Empty for any other call.
func (m AppModel) execCardCommand(p domain.PendingTool) string {
	tool, _, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || tool.Identity.Kind != domain.ToolKindLocal || tool.Identity.LocalOperation != "exec" {
		return ""
	}
	var call struct {
		Program        string   `json:"program"`
		Args           []string `json:"args"`
		Cwd            string   `json:"cwd"`
		Stdin          string   `json:"stdin"`
		TimeoutMS      int64    `json:"timeout_ms"`
		MaxOutputBytes int      `json:"max_output_bytes"`
		Why            string   `json:"why"`
	}
	raw := p.Call.Arguments.Bytes()
	if json.Unmarshal(raw, &call) != nil || strings.TrimSpace(call.Program) == "" {
		return ""
	}
	lines := []string{"$ " + singleLine(formatCommand(call.Program, call.Args))}
	if why := strings.TrimSpace(call.Why); why != "" {
		// The model's reason for the command, the first thing to weigh.
		lines = append(lines, m.Theme.Tf("approval.why", singleLine(why)))
	}
	if cwd := strings.TrimSpace(call.Cwd); cwd != "" && cwd != "." && cwd != "./" {
		lines = append(lines, m.Theme.Tf("approval.execCwd", singleLine(cwd)))
	}
	if call.TimeoutMS > 0 && call.TimeoutMS != execDefaultTimeoutMS {
		lines = append(lines, m.Theme.Tf("approval.execTimeout", formatDuration(call.TimeoutMS)))
	}
	if call.MaxOutputBytes > 0 {
		lines = append(lines, m.Theme.Tf("approval.execOutput", formatBytes(call.MaxOutputBytes)))
	}
	if call.Stdin != "" {
		lines = append(lines, m.Theme.T("approval.execStdin"))
		for _, line := range strings.Split(strings.TrimRight(call.Stdin, "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil {
		for key := range fields {
			switch key {
			case "program", "args", "cwd", "stdin", "timeout_ms", "max_output_bytes", "why":
			default:
				var indented bytes.Buffer
				_ = json.Indent(&indented, raw, "", "  ")
				return strings.Join(lines, "\n") + "\n\n" + indented.String()
			}
		}
	}
	return strings.Join(lines, "\n")
}

// destructiveRule names the destructive rule a pending command falls under.
func (m AppModel) destructiveRule(p domain.PendingTool) string {
	tool, args, known, err := application.ResolveToolCall(m.Header.State.ToolSnapshot, p.Call)
	if !known || err != nil || m.deps.Session == nil {
		return ""
	}
	return application.DestructiveReason(m.deps.ApprovalSettings, *m.deps.Session, tool.Identity, args)
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
