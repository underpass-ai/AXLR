package application

import (
	"context"
	"encoding/json"
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

// memoryReminderCalls is how many local tool calls make a request worth
// recording when it changed no file. File reads do not count, since a request
// that only read settled nothing durable, and neither do host bookkeeping and
// KMP reads: a session's startup alone makes several of them. A command does,
// since running a check or a test can settle an outcome.
const memoryReminderCalls = 5

// memoryReminder is the console's message; it leaves the judgement to the
// model and says how to decline.
const memoryReminder = MemoryReminderPrefix + " KMP is connected and this request did real work without recording anything. If it settled a decision, constraint, fix or outcome worth reusing, record it now with axlr_remember (kind, text and its evidence; the console uses the session's exact about and a stable idempotency key), then say in one sentence what you recorded. If nothing durable came out of it, or the user asked not to record, say so in one sentence without recording. The console asks this once per request."

// memoryReminderBridged is the reminder where axlr_remember is not offered.
const memoryReminderBridged = MemoryReminderPrefix + " KMP is connected and this request did real work without recording anything. If it settled a decision, constraint, fix or outcome worth reusing, record it now with kmp_write_memory through axlr_call_tool, under the session's exact about (axlr_session shows it), with its source evidence and a stable idempotency key, then say in one sentence what you recorded. If nothing durable came out of it, or the user asked not to record, say so in one sentence without recording. The console asks this once per request."

// remindMemory starts one console turn when a request ends with durable work
// and no memory write: models given KMP recall it reliably but record what
// they settle only now and then. It acts only where the model may write
// memory itself: KMP connected with kmp_write_memory, no ceremony (the
// console records those), a mode that does not refuse the write.
func remindMemory(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) (bool, error) {
	if s.Status() != domain.StatusComplete || !modelRecordsMemory(*s) {
		return false, nil
	}
	write, _, _ := memoryWriteTool(s.ToolSnapshot())
	messages := s.Messages()
	if len(messages) == 0 || messages[len(messages)-1].Role != root.RoleAssistant || len(messages[len(messages)-1].ToolCalls) > 0 {
		return false, nil
	}
	if answerAddressesMemory(messages[len(messages)-1].Content) {
		return false, nil
	}
	start := personRequest(messages)
	if start < 0 || !needsMemoryReminder(messages[start:], write) {
		return false, nil
	}
	next := *s
	reminder := memoryReminderBridged
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

// memoryWriteTool finds kmp_write_memory in the session's snapshot: the name
// the model calls it by and its identity for the mode's verdict.
func memoryWriteTool(snapshot []domain.AvailableTool) (root.ToolName, domain.ToolIdentity, bool) {
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin && tool.Identity.Plugin.PluginID == "kmp" && tool.Identity.Plugin.ToolName == "kmp_write_memory" {
			return tool.Definition.Name, tool.Identity, true
		}
	}
	return "", domain.ToolIdentity{}, false
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
// counts: they decided), and changed a file or made several local tool calls
// other than reads, searches and listings.
func needsMemoryReminder(request []root.Message, write root.ToolName) bool {
	local, changed := 0, false
	for _, message := range request[1:] {
		if message.Role == root.RoleUser && strings.HasPrefix(string(message.Content), MemoryReminderPrefix) {
			return false
		}
		for _, call := range message.ToolCalls {
			if countsForMemory(call.Name) {
				local++
			}
			switch call.Name {
			case write, HostStepDoneName, HostRememberName:
				return false
			case HostCallToolName:
				var bridged struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(call.Arguments.Bytes(), &bridged) == nil && (bridged.Name == string(write) || bridged.Name == "kmp_write_memory") {
					return false
				}
			case "local_write", "local_edit":
				changed = true
			}
		}
	}
	return changed || local >= memoryReminderCalls
}

// memoryAnswerPhrases are what a final answer says when the model already
// decided about recording, in English and Spanish, lower case with straight
// apostrophes. On 10 October 2026 claude-haiku-5.5 ended with "I did not
// record anything in KMP…" and was reminded anyway; it then recorded a
// low-value success_path, one extra request. Each phrase names recording
// itself, so "fixed the record type" or "el registro de errores" do not
// match. A model that ends without mentioning memory is still reminded.
var memoryAnswerPhrases = []string{
	// Declined.
	"did not record", "didn't record", "do not record", "don't record", "won't record", "will not record",
	"not recording", "nothing to record", "nothing worth recording", "nothing durable to record", "no need to record",
	"nothing to remember", "nothing to save in kmp", "nothing to store in kmp",
	"no registré", "no he registrado", "no registro nada", "nada que registrar", "nada que guardar",
	"no guardé", "no he guardado",
	// Recorded.
	"recorded in kmp", "recorded it in kmp", "recorded this in kmp", "recorded in memory", "recorded in project memory",
	"recorded a memory", "saved to kmp", "saved in kmp",
	"stored in kmp", "wrote to kmp", "written to kmp",
	"registrado en kmp", "registré en kmp", "lo registré", "registrado en la memoria", "guardado en kmp", "guardé en kmp",
}

// memoryAnchors are words one of which the answer must also contain: this
// workspace records traces and payloads too, and "the payload recorder did
// not record the body" or "no hay nada que guardar, el fichero ya está
// actualizado" are not about memory.
var memoryAnchors = []string{"kmp", "memory", "memoria", "remember", "durable", "duradero"}

// answerAddressesMemory reports whether a final answer already says what
// it recorded in KMP, or that it recorded nothing.
func answerAddressesMemory(answer root.Text) bool {
	text := strings.ToLower(strings.ReplaceAll(string(answer), "’", "'"))
	anchored := false
	for _, anchor := range memoryAnchors {
		anchored = anchored || strings.Contains(text, anchor)
	}
	if !anchored {
		return false
	}
	if strings.Contains(text, "axlr_remember") && strings.Contains(text, "recorded") {
		return true
	}
	for _, phrase := range memoryAnswerPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// countsForMemory reports whether a call counts toward memoryReminderCalls:
// a local tool call that is not a file read, a search or a listing, which
// change nothing.
func countsForMemory(name root.ToolName) bool {
	return strings.HasPrefix(string(name), "local_") && name != "local_read" && name != "local_search" && name != "local_list"
}
