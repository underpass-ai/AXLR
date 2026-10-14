package application

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// TurnRecord is what one turn of a session did, as the console writes it
// to the session's MADE step and, its results only, to KMP (decisions D4,
// E1, E5 and H8 of the lifecycle integration design): the prompt and the
// answer live in MADE alone; every side effect carries the model's why,
// the decision with the person's reason, its outcome, and for a command
// its exit code and the redacted tail of its output.
type TurnRecord struct {
	Turn        int            `json:"turn"`
	Prompt      string         `json:"prompt"`
	PromptIndex int            `json:"prompt_index"`
	Answer      string         `json:"answer,omitempty"`
	AnswerKind  string         `json:"answer_kind"`
	Actions     []TurnAction   `json:"actions,omitempty"`
	Memories    []TurnMemory   `json:"memories,omitempty"`
	Reads       int            `json:"reads"`
	Mode        string         `json:"mode"`
	Model       string         `json:"model"`
	About       string         `json:"about,omitempty"`
	Ceremony    string         `json:"ceremony,omitempty"`
	Cost        *TurnCost      `json:"cost,omitempty"`
	Interrupted bool           `json:"interrupted,omitempty"`
	Files       []string       `json:"files,omitempty"`
	Commands    []string       `json:"commands,omitempty"`
	StartedAt   time.Time      `json:"started_at,omitempty"`
	FinishedAt  time.Time      `json:"finished_at,omitempty"`
	Extra       map[string]any `json:"-"`
}

// Bounds of the record's texts.
const (
	turnExcerptBytes = 500
	turnOutputTail   = 4 << 10
)

// turnOutcomeOf classifies a tool result.
const (
	turnCompleted  = "completed"
	turnError      = "error"
	turnDenied     = "denied"
	turnCancelled  = "cancelled"
	turnRejected   = "rejected"
	turnOverBudget = "over_budget"
	turnUnknown    = "unknown"
)

// sideEffectTools are the calls the record lists one by one; every other
// call is a read, counted.
var sideEffectTools = map[root.ToolName]bool{"local_write": true, "local_edit": true, "local_exec": true, HostCallToolName: true, HostRunToolName: true, HostRememberName: true, HostStepDoneName: true}

// secretPattern redacts values that look like keys or tokens (decision E11).
var secretPattern = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,}|AKIA[A-Z0-9]{12,}|xox[abp]-[A-Za-z0-9-]{10,}|Bearer\s+[A-Za-z0-9._-]{8,}|\b[A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD)=\S+)`)

// Redact replaces what looks like a secret with [redacted].
func Redact(text string) string {
	return secretPattern.ReplaceAllString(text, "[redacted]")
}

// personTyped is what the person typed: a prompt without the console notes
// appended to it ("\n\n[AXLR · recall] …", a ceremony step), which the
// record would otherwise repeat in MADE, in LastRecord and in /session.
func personTyped(content string) string {
	if at := strings.Index(content, "\n\n[AXLR"); at >= 0 {
		return content[:at]
	}
	return content
}

// BuildTurnRecord reads the turn that starts at the person's prompt at
// index from the transcript: console messages ([AXLR …]) belong to it, as
// the memory reminder counts them.
func BuildTurnRecord(s domain.Session, index, turn int, about string, cost *TurnCost) TurnRecord {
	state := s.Export()
	messages := state.Messages
	record := TurnRecord{Turn: turn, PromptIndex: index, Mode: string(s.Mode()), Model: string(state.Model), About: about, Cost: cost, AnswerKind: "answered"}
	if index < 0 || index >= len(messages) {
		return record
	}
	record.Prompt = Redact(excerpt(personTyped(string(messages[index].Content)), turnExcerptBytes))
	record.StartedAt = messageTime(state, index)
	decisions := map[root.ToolCallID]domain.PendingTool{}
	for _, activity := range state.Activity {
		decisions[activity.Call.ID] = activity
	}
	results := map[root.ToolCallID]int{}
	for i := index; i < len(messages); i++ {
		if messages[i].Role == root.RoleTool {
			results[messages[i].ToolCallID] = i
		}
	}
	answer := ""
	bookkeeping := false
	for i := index + 1; i < len(messages); i++ {
		message := messages[i]
		if message.Role == root.RoleUser {
			if !strings.HasPrefix(string(message.Content), "[AXLR") {
				break // the next turn
			}
			// What the model says after a console message (the memory
			// reminder, a budget note) is bookkeeping, not its answer to
			// the person; the person's answer is the one before it.
			bookkeeping = true
		}
		if message.Role == root.RoleAssistant && len(message.ToolCalls) == 0 && strings.TrimSpace(string(message.Content)) != "" && (!bookkeeping || answer == "") {
			answer = string(message.Content)
		}
		for _, call := range message.ToolCalls {
			if !sideEffectTools[call.Name] {
				record.Reads++
				continue
			}
			action := TurnAction{Tool: string(call.Name), Why: CallWhy(call.Arguments), CallID: string(call.ID)}
			action.Target = Redact(actionTarget(call))
			if pending, ok := decisions[call.ID]; ok {
				action.Decision = string(pending.Decision)
			}
			resultText := ""
			if at, ok := results[call.ID]; ok {
				resultText = string(messages[at].Content)
				action.OccurredAt = messageTime(state, at)
			}
			action.Outcome, action.Reason = classifyOutcome(resultText)
			if call.Name == "local_exec" || call.Name == HostRunToolName {
				action.ExitCode, action.OutputTail = execResult(resultText)
				if action.ExitCode != nil && action.Outcome == turnCompleted && *action.ExitCode != 0 {
					action.Outcome = turnError
				}
			}
			switch call.Name {
			case "local_write", "local_edit":
				if action.Outcome == turnCompleted {
					record.Files = appendUnique(record.Files, argumentString(call.Arguments, "path"))
				}
			case "local_exec":
				if action.Outcome == turnCompleted || action.Outcome == turnError {
					if action.ExitCode != nil {
						record.Commands = append(record.Commands, action.Target+" → exit "+itoa(*action.ExitCode))
					}
				}
			case HostRememberName:
				if about, key, ok := rememberAccepted(resultText); ok {
					record.Memories = append(record.Memories, TurnMemory{About: about, Key: key})
				}
			case HostCallToolName:
				if name, arguments := memoryCall(call); isMemoryWrite(state.ToolSnapshot, name) && memoryWriteAccepted(resultText) {
					record.Memories = append(record.Memories, TurnMemory{About: argumentString(arguments, "about"), Key: argumentString(arguments, "idempotency_key")})
				}
			}
			record.Actions = append(record.Actions, action)
		}
	}
	if run, live := s.Ceremony(); live {
		record.Ceremony = run.Instance
	}
	record.Answer = Redact(excerpt(answer, turnExcerptBytes))
	switch {
	case s.Status() == domain.StatusInterrupted:
		record.AnswerKind, record.Interrupted = "paused", true
	case len(record.Files) > 0:
		record.AnswerKind = "changed"
	case strings.HasSuffix(strings.TrimSpace(answer), "?"):
		record.AnswerKind = "asked"
	}
	if len(messages) > 0 {
		record.FinishedAt = messageTime(state, len(messages)-1)
	}
	return record
}

// HasResults reports whether the turn left something KMP should hold: a
// file, a command, an external call, a memory. An answer from reading
// leaves nothing.
func (r TurnRecord) HasResults() bool {
	return len(r.Files) > 0 || len(r.Commands) > 0 || len(r.Memories) > 0 || len(r.Actions) > 0
}

// Summary is the record's one-line English summary for KMP.
func (r TurnRecord) Summary() string {
	parts := []string{}
	if n := len(r.Files); n > 0 {
		parts = append(parts, plural(n, "file")+" written ("+strings.Join(firstFew(r.Files, 4), ", ")+")")
	}
	if n := len(r.Commands); n > 0 {
		parts = append(parts, plural(n, "command")+" run")
	}
	if n := len(r.Memories); n > 0 {
		parts = append(parts, plural(n, "memory")+" recorded")
	}
	denied := 0
	for _, action := range r.Actions {
		if action.Outcome == turnDenied {
			denied++
		}
	}
	if denied > 0 {
		parts = append(parts, plural(denied, "call")+" denied by the person")
	}
	if len(parts) == 0 {
		parts = append(parts, "no file, command or memory")
	}
	text := "Turn " + itoa(r.Turn) + ": " + strings.Join(parts, "; ") + "."
	if r.Interrupted {
		text += " Interrupted."
	}
	return text
}

// Evidence is one line per action (decision E8): the tool, its target, the
// why, the decision, the outcome and for a command its exit code.
func (r TurnRecord) Evidence(stepRef string) []string {
	lines := []string{stepRef}
	for _, action := range r.Actions {
		line := action.Tool + " " + action.Target
		if action.Why != "" {
			line += " — why: " + action.Why
		}
		line += " — " + action.Decision + ", " + action.Outcome
		if action.Reason != "" {
			line += " (" + action.Reason + ")"
		}
		if action.ExitCode != nil {
			line += ", exit " + itoa(*action.ExitCode)
		}
		lines = append(lines, excerpt(line, 1000))
		if len(lines) == 8 {
			break
		}
	}
	return lines
}

// JSON is the record for the MADE step output and the sidecar.
func (r TurnRecord) JSON() string {
	data, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// BoundedJSON is the record as LastRecord keeps it: valid JSON of at most
// limit bytes. Cutting the JSON (as bounded did until 14 October 2026)
// left it over the limit and unparseable, Validate refused the instance
// and every later turn of the session failed. What /session needs least
// goes first: the output tails, the whys, then the actions, the prompt and
// the answer; a record still too large keeps its numbers alone.
func (r TurnRecord) BoundedJSON(limit int) string {
	if text := r.JSON(); len(text) <= limit {
		return text
	}
	r.Actions = append([]TurnAction(nil), r.Actions...)
	for i := range r.Actions {
		r.Actions[i].OutputTail = ""
	}
	if text := r.JSON(); len(text) <= limit {
		return text
	}
	for i := range r.Actions {
		r.Actions[i].Why = ""
	}
	if text := r.JSON(); len(text) <= limit {
		return text
	}
	for len(r.Actions) > 0 {
		r.Actions = r.Actions[:len(r.Actions)/2]
		if text := r.JSON(); len(text) <= limit {
			return text
		}
	}
	r.Prompt, r.Answer, r.Files, r.Commands, r.Memories = "", "", nil, nil, nil
	if text := r.JSON(); len(text) <= limit {
		return text
	}
	minimal, _ := json.Marshal(map[string]any{"turn": r.Turn, "prompt_index": r.PromptIndex, "answer_kind": r.AnswerKind, "truncated": true})
	return string(minimal)
}

// Output is the record as the MADE step's output object.
func (r TurnRecord) Output() map[string]any {
	var output map[string]any
	if json.Unmarshal([]byte(r.JSON()), &output) != nil {
		return map[string]any{"turn": r.Turn}
	}
	for key, value := range r.Extra {
		output[key] = value
	}
	return output
}

// actionTarget is the digest of a call's arguments: a path, a command line,
// the plugin tool, the forged tool.
func actionTarget(call root.ToolCall) string {
	switch call.Name {
	case "local_write":
		return argumentString(call.Arguments, "path") + " (" + argumentString(call.Arguments, "mode") + ")"
	case "local_edit":
		return argumentString(call.Arguments, "path") + " (edit)"
	case "local_exec":
		var exec struct {
			Program string   `json:"program"`
			Args    []string `json:"args"`
			Cwd     string   `json:"cwd"`
		}
		_ = json.Unmarshal(call.Arguments.Bytes(), &exec)
		line := strings.Join(append([]string{exec.Program}, exec.Args...), " ")
		if exec.Cwd != "" && exec.Cwd != "." {
			line += " (in " + exec.Cwd + ")"
		}
		return excerpt(line, 300)
	case HostCallToolName, HostRunToolName:
		return argumentString(call.Arguments, "name")
	case HostRememberName:
		return argumentString(call.Arguments, "kind") + ": " + excerpt(argumentString(call.Arguments, "text"), 120)
	case HostStepDoneName:
		return "step hand-back"
	}
	return string(call.Name)
}

// classifyOutcome reads a tool result: the console's refusals are plain
// text, the runtime's results a JSON envelope whose "error" is null when
// the call succeeded. It is the one reader of a result's outcome: the turn
// record, the shortened closed-turn writes and Jev's final check all use
// it. On 14 October 2026 the final check took every result holding
// `"error"` for a failure, `"error":null` included, and Jev doubted three
// of four correct answers.
func classifyOutcome(result string) (outcome, reason string) {
	switch {
	case result == "":
		return turnUnknown, ""
	case strings.HasPrefix(result, "tool call denied by user"):
		_, reason, _ = strings.Cut(result, ": ")
		return turnDenied, strings.TrimSpace(reason)
	case strings.HasPrefix(result, "tool call cancelled"):
		// A pending call the person cancelled, or one cancelled while it ran.
		return turnCancelled, ""
	case strings.HasPrefix(result, "tool execution failed; effect unknown") || strings.HasPrefix(result, "tool execution started; effect unknown"):
		return turnError, "effect unknown"
	case strings.HasPrefix(result, "not run: over the turn's tool-call budget"):
		return turnOverBudget, ""
	case strings.HasPrefix(result, "invalid tool invocation rejected") || strings.HasPrefix(result, "unknown tool"):
		return turnRejected, excerpt(result, 200)
	}
	var envelope struct {
		Status string `json:"status"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Output json.RawMessage `json:"output"`
	}
	if json.Unmarshal([]byte(result), &envelope) != nil {
		// A host tool's own JSON, or plain text: an error field says so.
		var host struct {
			Error    string `json:"error"`
			Accepted *bool  `json:"accepted"`
		}
		if json.Unmarshal([]byte(result), &host) == nil && host.Error != "" {
			return turnError, excerpt(host.Error, 200)
		}
		return turnCompleted, ""
	}
	if envelope.Status == "" {
		if envelope.Error != nil {
			return turnError, envelope.Error.Code
		}
		var host struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(result), &host) == nil && host.Error != "" {
			return turnError, excerpt(host.Error, 200)
		}
		return turnCompleted, ""
	}
	if envelope.Status != "completed" || envelope.Error != nil {
		reason = envelope.Status
		if envelope.Error != nil {
			reason = envelope.Error.Code
		}
		return turnError, reason
	}
	var plugin struct {
		IsError bool `json:"is_error"`
	}
	if json.Unmarshal(envelope.Output, &plugin) == nil && plugin.IsError {
		return turnError, "plugin error"
	}
	return turnCompleted, ""
}

// execResult reads a command's exit code and the redacted tail of its
// output from the runtime envelope.
func execResult(result string) (*int, string) {
	var envelope struct {
		Output struct {
			ExitCode *int   `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"output"`
	}
	if json.Unmarshal([]byte(result), &envelope) != nil || envelope.Output.ExitCode == nil {
		return nil, ""
	}
	text := strings.TrimSpace(envelope.Output.Stdout)
	if stderr := strings.TrimSpace(envelope.Output.Stderr); stderr != "" {
		if text != "" {
			text += "\n"
		}
		text += stderr
	}
	if len(text) > turnOutputTail {
		text = text[len(text)-turnOutputTail:]
		// The cut can fall inside a character: the tail starts at the next
		// one, so the record stays valid UTF-8.
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
		if i := strings.IndexByte(text, '\n'); i >= 0 && i < 200 {
			text = text[i+1:]
		}
	}
	return envelope.Output.ExitCode, Redact(text)
}

func messageTime(state domain.SessionState, index int) time.Time {
	if index >= 0 && index < len(state.MessageTimes) {
		return state.MessageTimes[index]
	}
	return time.Time{}
}

func excerpt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func appendUnique(values []string, value string) []string {
	if value == "" || contains(values, value) {
		return values
	}
	return append(values, value)
}

func firstFew(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return append(append([]string(nil), values[:limit]...), "…")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "y") {
		return itoa(n) + " " + strings.TrimSuffix(noun, "y") + "ies"
	}
	return itoa(n) + " " + noun + "s"
}

func itoa(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}
