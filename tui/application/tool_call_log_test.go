package application

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolCallRecorder keeps what the tool-call log heard.
type toolCallRecorder struct{ records []ToolCallRecord }

func (r *toolCallRecorder) ToolCallFinished(record ToolCallRecord) {
	r.records = append(r.records, record)
}

// On 12 November 2026 a bot called axlr_remember ten times, each refused by
// its team's memory policy, and neither the app log nor the trace told the
// operator: the log had no line for a tool call and the trace classed the
// refusal as the machinery's failure. The refusal now reaches the log
// whole, and the trace classes it as the call's own answer.
func TestToolCallLogHearsARefusedTeamMemoryWriteWhole(t *testing.T) {
	// The team's bot, with axlr_remember offered as the turn offers it.
	s := teamSession(t, false)
	if err := s.BeginTurn("record what we found", append(memoryTools(t), RememberTool())); err != nil {
		t.Fatal(err)
	}
	teammate := "project:qa.qa-explorador"
	remember := toolCall(t, "m1", HostRememberName, `{"kind":"observation","text":"the charter ran","evidence":"run 3","about":"`+teammate+`"}`)
	if err := s.CompleteAssistant(assistant("", remember)); err != nil {
		t.Fatal(err)
	}
	trace := &diagnosticCollector{}
	log := &toolCallRecorder{}
	memory := &rememberRecorder{answer: map[string]any{"accepted": true}}
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Diagnostics: trace, Continue: ContinueTurnUseCase{Store: store, Remember: memory, ToolCalls: log, SessionLabels: &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}}}
	if err := u.resolveOne(context.Background(), &s, "m1", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if len(memory.requests) != 0 {
		t.Fatal("KMP took a write to a teammate's about")
	}
	if len(log.records) != 1 {
		t.Fatalf("%d records", len(log.records))
	}
	record := log.records[0]
	want := teammate + " is a teammate's or another's, which the execution's policy lets this session read and not write"
	if record.Session != s.ID() || record.Call != "m1" || record.Tool != HostRememberName || record.PluginTool != "" || record.Decision != domain.DecisionApprove || !record.Failed || !strings.Contains(record.Error, want) {
		t.Fatalf("record %+v", record)
	}
	if strings.Contains(record.Error, "the charter ran") || strings.Contains(record.Error, "run 3") {
		t.Fatalf("the record carries the call's arguments: %q", record.Error)
	}
	completed := 0
	for _, event := range trace.events {
		if event.Stage == DiagnosticToolCompleted {
			completed++
			if event.ErrorClass != DiagnosticErrorToolResult {
				t.Fatalf("tool_completed class %q", event.ErrorClass)
			}
		}
	}
	if completed != 1 {
		t.Fatalf("%d tool_completed records", completed)
	}
}

// Every finished call is heard, however it ended: run, refused before it
// ran, denied by the person. A refused call never ran and takes no time.
func TestToolCallLogHearsEveryFinishedCall(t *testing.T) {
	s := queued(t, call("a", "read"), call("bad", "unknown"), call("b", "read"))
	store := &memoryStore{}
	log := &toolCallRecorder{}
	u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: "fine"}, nil
	}), Continue: ContinueTurnUseCase{Store: store, ToolCalls: log}}
	if err := u.resolveOne(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	// The unknown call in front of b is refused when b is decided.
	if err := u.resolveOne(context.Background(), &s, "bad", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if err := u.resolveOne(context.Background(), &s, "b", domain.DecisionDeny, nil); err != nil {
		t.Fatal(err)
	}
	if len(log.records) != 3 {
		t.Fatalf("records %+v", log.records)
	}
	ok, refused, denied := log.records[0], log.records[1], log.records[2]
	if ok.Call != "a" || ok.Tool != "read" || ok.Failed || ok.Error != "" || ok.Decision != domain.DecisionApprove || ok.Duration <= 0 {
		t.Fatalf("ok %+v", ok)
	}
	if refused.Call != "bad" || !refused.Failed || refused.Decision != domain.DecisionDeny || refused.Duration != 0 || !strings.Contains(refused.Error, `unknown tool "unknown" rejected`) {
		t.Fatalf("refused %+v", refused)
	}
	if denied.Call != "b" || !denied.Failed || denied.Decision != domain.DecisionDeny || denied.Error != "tool call denied by user" {
		t.Fatalf("denied %+v", denied)
	}
}

// A call past the turn's budget is answered "over budget" and heard.
func TestToolCallLogHearsACallOverTheBudget(t *testing.T) {
	s := queued(t, call("a", "read"), call("b", "read"))
	store := &memoryStore{}
	log := &toolCallRecorder{}
	admit := headAdmission{Store: store, TurnLimit: 1, ToolCalls: log}
	if err := (ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: "fine"}, nil
	}), Continue: ContinueTurnUseCase{Store: store, TurnToolCalls: 1, ToolCalls: log}}).resolveOne(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if err := rejectUnknown(context.Background(), &s, admit, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(log.records) != 2 || log.records[1].Call != "b" || !log.records[1].Failed || log.records[1].Duration != 0 {
		t.Fatalf("records %+v", log.records)
	}
}

// Without a port nothing is recorded and nothing breaks.
func TestToolCallLogIsOptional(t *testing.T) {
	s := queued(t, call("a", "read"))
	logFinishedTool(nil, &s, "a", time.Time{})
	log := &toolCallRecorder{}
	// A call still waiting has no outcome to record.
	logFinishedTool(log, &s, "a", time.Time{})
	logFinishedTool(log, nil, "a", time.Time{})
	if len(log.records) != 0 {
		t.Fatalf("records %+v", log.records)
	}
}

// A failure's text goes on one line, whole up to its bound and cut on a
// character boundary past it.
func TestToolCallErrorTextFlattensAndBounds(t *testing.T) {
	if got := ToolCallErrorText("\n  expired SSO session  \r\n\nsecond line\n", 100); got != "expired SSO session | second line" {
		t.Fatalf("flattened %q", got)
	}
	// A longer text keeps its beginning and its end: here a command's
	// standard error, last in the runtime's answer.
	long := `{"status":"completed","output":{"stdout":"` + strings.Repeat("é", 2000) + `","stderr":"permission denied: /etc/shadow"}}`
	got := ToolCallErrorText(long, MaxToolCallErrorBytes)
	if len(got) > MaxToolCallErrorBytes || !strings.HasPrefix(got, `{"status":"completed"`) || !strings.HasSuffix(got, `"stderr":"permission denied: /etc/shadow"}}`) || strings.Count(got, " … ") != 1 || !utf8.ValidString(got) {
		t.Fatalf("bounded to %d bytes, valid %v: %q", len(got), utf8.ValidString(got), got)
	}
	// Bounding again, tighter, keeps one gap between the same ends.
	if again := ToolCallErrorText(got, 1536); len(again) > 1536 || strings.Count(again, " … ") != 1 || !strings.HasSuffix(again, "/etc/shadow\"}}") {
		t.Fatalf("bounded again %q", again)
	}
	if got := ToolCallErrorText("abcdef", 2); got != "ab" {
		t.Fatalf("tiny bound %q", got)
	}
	if got := ToolCallErrorText("", MaxToolCallErrorBytes); got != "" {
		t.Fatalf("empty %q", got)
	}
}

// The plugin's tool is named however the model reached it.
func TestCallPluginToolNamesThePluginsTool(t *testing.T) {
	snapshot := []domain.AvailableTool{hostPlugin(t, "mcp_0123456789abcdef", "kmp", "kmp_write_memory")}
	if got := CallPluginTool(snapshot, toolCall(t, "c", "mcp_0123456789abcdef", `{}`)); got != "kmp/kmp_write_memory" {
		t.Fatalf("direct %q", got)
	}
	if got := CallPluginTool(snapshot, toolCall(t, "c", "read", `{}`)); got != "" {
		t.Fatalf("unknown %q", got)
	}
}
