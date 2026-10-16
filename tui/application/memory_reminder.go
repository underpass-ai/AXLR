package application

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// MemoryReminderPrefix opens the console's reminder to record what a request
// settled in KMP. A request that already received it is not reminded again,
// so the reminder costs at most one extra model turn per request. The
// reminder is a user message the console wrote, not the person: the
// transcript shows messages with this prefix as a console line.
const MemoryReminderPrefix = "[AXLR · memory]"

// memoryReminder is the console's message; it leaves the judgement to the
// model and says how to decline.
const memoryReminder = MemoryReminderPrefix + " Memory is connected and this request did real work without recording anything. If it settled a decision, constraint, fix or outcome worth reusing, record it now with axlr_remember (kind, text, summary_en and its evidence; the console uses the session's exact about and a stable idempotency key), then say in one sentence what you recorded. If nothing durable came out of it, or the user asked not to record, say so in one sentence without recording. The console asks this once per request."

// memoryReminderBridgedFormat is the reminder where axlr_remember is not
// offered; %s is the memory engine's write, by its own name.
const memoryReminderBridgedFormat = MemoryReminderPrefix + " Memory is connected and this request did real work without recording anything. If it settled a decision, constraint, fix or outcome worth reusing, record it now with %s through axlr_call_tool, under the session's exact about (axlr_session shows it), with its source evidence and a stable idempotency key, then say in one sentence what you recorded. If nothing durable came out of it, or the user asked not to record, say so in one sentence without recording. The console asks this once per request."

// remindMemory starts one console turn when a request ends with durable work
// and no memory write: models given KMP recall it reliably but record what
// they settle only now and then. It acts only where the model may write
// memory itself: KMP connected with kmp_write_memory, no ceremony (the
// console records those), a mode that does not refuse the write.
func remindMemory(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) (bool, error) {
	if s.Status() != domain.StatusComplete || !modelRecordsMemory(*s) {
		return false, nil
	}
	if u.Session.settles() && u.Session.recordsTurn(*s) {
		// Jev decides what the turn settled (decision D5), at the turn's
		// record: without one (MADE unavailable, the instance closed) Jev is
		// never asked and the reminder is the only prompt to record.
		return false, nil
	}
	messages := s.Messages()
	if len(messages) == 0 || messages[len(messages)-1].Role != root.RoleAssistant || len(messages[len(messages)-1].ToolCalls) > 0 {
		return false, nil
	}
	if answerAddressesMemory(messages[len(messages)-1].Content) {
		return false, nil
	}
	start := personRequest(messages)
	if start < 0 || !needsMemoryReminder(messages[start:], s.ToolSnapshot()) {
		return false, nil
	}
	next := *s
	_, write, _ := memoryWriteTool(s.ToolSnapshot())
	reminder := fmt.Sprintf(memoryReminderBridgedFormat, write.Plugin.ToolName)
	if _, remembers := findTool(s, HostRememberName); remembers {
		reminder = memoryReminder
	}
	if err := next.BeginTurn(root.Text(reminder), next.ToolSnapshot()); err != nil {
		return false, err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}

// modelRecordsMemory reports whether the model itself records in KMP: the
// mode does not refuse kmp_write_memory and no ceremony runs, whose outcome
// the console records.
func modelRecordsMemory(s domain.Session) bool {
	if _, live := s.Ceremony(); live || s.Mode().StartsCeremony() || s.Mode() == domain.ModeTask {
		return false
	}
	_, identity, ok := memoryWriteTool(s.ToolSnapshot())
	if !ok {
		return false
	}
	verdict, _ := s.Mode().Judge(identity, root.JSONValue{})
	return verdict != domain.VerdictDeny
}

// memoryWriteTool finds the memory engine's write in the session's
// snapshot (its surface says which tool it is): the name the model calls
// it by and its identity for the mode's verdict.
func memoryWriteTool(snapshot []domain.AvailableTool) (root.ToolName, domain.ToolIdentity, bool) {
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin && tool.Surface.MemoryWrite {
			return tool.Definition.Name, tool.Identity, true
		}
	}
	return "", domain.ToolIdentity{}, false
}

// isMemoryWrite reports whether a name the model called, directly or
// through axlr_call_tool, is the memory engine's write: by the name the
// snapshot gives it or by the engine's own.
func isMemoryWrite(snapshot []domain.AvailableTool, name string) bool {
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin && tool.Surface.MemoryWrite && (string(tool.Definition.Name) == name || string(tool.Identity.Plugin.ToolName) == name) {
			return true
		}
	}
	return false
}

// personRequest is the index of the last message the person wrote. Console
// messages, which all start with "[AXLR", belong to the request before them.
func personRequest(messages []root.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == root.RoleUser && !strings.HasPrefix(string(messages[i].Content), "[AXLR") {
			return i
		}
	}
	return -1
}

// needsMemoryReminder reads one request: it was not reminded yet, ran no
// ceremony step, attempted no memory write (an attempt the person denied
// counts: they decided), and changed a file or ran a command (decision F2
// of the lifecycle integration design: a check or a test can settle an
// outcome; reads, searches, listings, bookkeeping and KMP reads never
// do). A write or a command counts only when its result says it
// completed: on 10 October 2026 the person denied the only local_edit of a
// request and the console still reminded the model that the request "did
// real work".
func needsMemoryReminder(request []root.Message, snapshot []domain.AvailableTool) bool {
	results := map[root.ToolCallID]string{}
	for _, message := range request {
		if message.Role == root.RoleTool {
			results[message.ToolCallID] = string(message.Content)
		}
	}
	ran, changed := false, false
	for _, message := range request[1:] {
		if message.Role == root.RoleUser && strings.HasPrefix(string(message.Content), MemoryReminderPrefix) {
			return false
		}
		for _, call := range message.ToolCalls {
			if isMemoryWrite(snapshot, string(call.Name)) {
				return false
			}
			switch call.Name {
			case HostStepDoneName, HostRememberName:
				return false
			case HostCallToolName:
				var bridged struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(call.Arguments.Bytes(), &bridged) == nil && isMemoryWrite(snapshot, bridged.Name) {
					return false
				}
			case "local_write", "local_edit":
				changed = changed || writeSucceeded(results[call.ID])
			case "local_exec", HostRunToolName:
				ran = ran || commandRan(results[call.ID])
			}
		}
	}
	return changed || ran
}

// commandRan reports a command whose result says it ran: not denied,
// cancelled or refused before starting.
func commandRan(result string) bool {
	if result == "" {
		return false
	}
	lower := strings.ToLower(result)
	for _, refused := range []string{"denied by user", "tool call cancelled", "tool call denied", "give why", "effect unknown"} {
		if strings.Contains(lower, refused) {
			return false
		}
	}
	var envelope struct {
		Status string          `json:"status"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(result), &envelope) == nil {
		if envelope.Status != "" && envelope.Status != "completed" && envelope.Status != "ok" {
			return false
		}
		// A host refusal (hostFailure) is {"error": ...} with no status: a
		// refused axlr_run_tool, an unknown tool or invalid arguments never
		// ran its program.
		if envelope.Status == "" && len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			return false
		}
	}
	return true
}

// answerAddressesMemory reports whether a final answer already says what
// it recorded in KMP, or that it recorded nothing: one of its sentences
// names memory and a recording verb. On 10 October 2026 claude-haiku-5.5
// ended with "I did not record anything in KMP…" and was reminded anyway;
// it then recorded a low-value success_path, one extra request. A list of
// set phrases missed the next answer ("I didn't write a KMP memory
// either"), so the check reads sentences rather than phrases. Identifiers
// (kmp_write_memory, memory_reminder.go) are left out first, so an answer
// about the memory code itself does not count; a model that ends without
// mentioning memory is still reminded.
func answerAddressesMemory(answer root.Text) bool {
	text := strings.ToLower(strings.ReplaceAll(string(answer), "’", "'"))
	if strings.Contains(text, "axlr_remember") && strings.Contains(text, "recorded") {
		return true
	}
	for _, sentence := range memorySentence.Split(memoryIdentifier.ReplaceAllString(text, " "), -1) {
		if memoryAnchor.MatchString(sentence) && memoryVerb.MatchString(sentence) {
			return true
		}
	}
	return false
}

var (
	memorySentence   = regexp.MustCompile(`[.!?;:\n]+(\s|$)`)
	memoryIdentifier = regexp.MustCompile(`\S*[_/]\S*|\S+\.(go|md|json)\b`)
	// memoryAnchor names memory itself: this workspace also records traces
	// and payloads, and "the payload recorder did not record the body" or
	// "no hay nada que guardar, el fichero ya está actualizado" are not
	// about memory.
	memoryAnchor = regexp.MustCompile(`\b(kmp|memory|memories|memoria|remember|durable|duradero)\b`)
	memoryVerb   = regexp.MustCompile(`\b(record|records|recorded|recording|write|wrote|written|writing|save|saved|saving|store|stored|storing|remember|remembered|registr[a-z]*)\b|\bguard(é|ado|ada|ar|o)`)
)
