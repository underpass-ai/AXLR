package terminal

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// commandAllows holds the local commands the person allowed with S on their
// card for the rest of one session: the exact program, arguments, working
// directory and standard input, in this console's memory only. Nothing is
// saved, nothing matches by prefix or by program name: on 10 October 2026 a
// proposal to allow git by name was rejected, since `git diff --output`,
// `git -C` and `git -c` can write anywhere. On 12 October 2026 one /review
// question raised four cards for read-only git commands.
type commandAllows struct {
	mu       sync.Mutex
	session  domain.SessionID
	commands map[string]bool
}

// allow adds a local exec call's command for the session; false when its
// arguments cannot be read.
func (a *commandAllows) allow(session domain.SessionID, arguments root.JSONValue) bool {
	key, ok := execAllowKey(arguments.Bytes())
	if !ok || session == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session != session {
		a.session, a.commands = session, map[string]bool{}
	}
	a.commands[key] = true
	return true
}

func (a *commandAllows) allows(session domain.SessionID, arguments root.JSONValue) bool {
	key, ok := execAllowKey(arguments.Bytes())
	if !ok {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session == session && a.commands[key]
}

// reset forgets every allowed command, as when the console shows another
// session.
func (a *commandAllows) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.session, a.commands = "", nil
}

// execAllowKey is what an allowed command must repeat exactly: every
// argument of the call but its timeout and output limit, with the
// workspace root written one way ("", "." and "./").
func execAllowKey(arguments []byte) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	var fields map[string]any
	if decoder.Decode(&fields) != nil {
		return "", false
	}
	if program, _ := fields["program"].(string); strings.TrimSpace(program) == "" {
		return "", false
	}
	delete(fields, "timeout_ms")
	delete(fields, "max_output_bytes")
	if cwd, ok := fields["cwd"].(string); ok && (cwd == "" || cwd == "." || cwd == "./") {
		delete(fields, "cwd")
	}
	if stdin, ok := fields["stdin"].(string); ok && stdin == "" {
		delete(fields, "stdin")
	}
	if args, ok := fields["args"].([]any); ok && len(args) == 0 {
		delete(fields, "args")
	}
	key, err := json.Marshal(fields)
	return string(key), err == nil
}

// allowingPolicy is the console's approval policy: the saved settings, and
// the commands allowed for the session.
type allowingPolicy struct {
	settings application.ToolApprovalPolicyPort
	allowed  *commandAllows
}

var _ application.CallApprovalPort = allowingPolicy{}

func (p allowingPolicy) AutoApproves(id domain.ToolIdentity) bool {
	return p.settings != nil && p.settings.AutoApproves(id)
}

func (p allowingPolicy) ApprovesCall(session domain.SessionID, id domain.ToolIdentity, arguments root.JSONValue) bool {
	return id.Kind == domain.ToolKindLocal && id.LocalOperation == "exec" && p.allowed.allows(session, arguments)
}

// withCommandAllows puts the session's allowed commands in front of the
// policy of every use case that approves calls.
func (m *AppModel) withCommandAllows() {
	m.allowed = &commandAllows{}
	m.deps.Start.Approval = allowingPolicy{settings: m.deps.Start.Approval, allowed: m.allowed}
	m.deps.Resolve.Approval = allowingPolicy{settings: m.deps.Resolve.Approval, allowed: m.allowed}
	m.deps.Agent.Approval = allowingPolicy{settings: m.deps.Agent.Approval, allowed: m.allowed}
}
