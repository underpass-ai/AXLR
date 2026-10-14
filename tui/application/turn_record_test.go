package application

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// recordedTurn is one turn with a write, a command, a read, a memory and a
// denied call, as the transcript keeps them.
func recordedTurn(t *testing.T) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	recordTurnOn(t, &s)
	return s
}

// recordTurnOn runs the recorded turn's calls and answer on a session whose
// prompt is already appended.
func recordTurnOn(t *testing.T, s *domain.Session) {
	t.Helper()
	calls := []root.ToolCall{
		{ID: "c1", Name: "local_write", Arguments: mustObject(t, `{"path":"hola.txt","content":"hola","mode":"create","why":"create the greeting file"}`)},
		{ID: "c2", Name: "local_read", Arguments: mustObject(t, `{"path":"hola.txt"}`)},
		{ID: "c3", Name: "local_exec", Arguments: mustObject(t, `{"program":"cat","args":["hola.txt"],"why":"check the file"}`)},
		{ID: "c4", Name: HostRememberName, Arguments: mustObject(t, `{"kind":"decision","text":"Greetings live in hola.txt","evidence":"hola.txt"}`)},
		{ID: "c5", Name: "local_exec", Arguments: mustObject(t, `{"program":"rm","args":["-rf","/"],"why":"clean up"}`)},
	}
	if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	outcomes := []struct {
		decision domain.ToolDecision
		outcome  domain.ToolOutcome
	}{
		{domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"protocol_version":1,"tool":"write","status":"completed","output":{"written_bytes":4},"error":null}`}},
		{domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"protocol_version":1,"tool":"read","status":"completed","output":{"content":"hola"},"error":null}`}},
		{domain.DecisionApprove, domain.ToolOutcome{Content: `{"protocol_version":1,"tool":"exec","status":"completed","output":{"exit_code":0,"stdout":"hola","stderr":"","captured_bytes":4},"error":null}`}},
		{domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"accepted":true,"about":"project:Lab","idempotency_key":"axlr:remember:abc","ref":"project:Lab:entry:decision:x"}`}},
		{domain.DecisionDeny, domain.ToolOutcome{Content: "tool call denied by user: never delete the root", IsError: true}},
	}
	for i, outcome := range outcomes {
		if err := s.RecordToolOutcome(calls[i].ID, outcome.decision, outcome.outcome); err != nil {
			t.Fatalf("outcome %d: %v", i, err)
		}
	}
	if err := s.CompleteAssistant(assistant("Hecho: hola.txt creado y comprobado.")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTurnRecordListsSideEffectsWithTheirWhyDecisionAndOutcome(t *testing.T) {
	s := recordedTurn(t)
	record := BuildTurnRecord(s, 0, 1, "project:Lab", &TurnCost{Requests: 2, USD: 0.01})
	if record.Prompt != "añade hola.txt y comprueba" || record.Answer != "Hecho: hola.txt creado y comprobado." || record.AnswerKind != "changed" || record.Reads != 1 || record.Turn != 1 {
		t.Fatalf("record %+v", record)
	}
	if len(record.Actions) != 4 {
		t.Fatalf("actions %+v", record.Actions)
	}
	write, exec, remember, denied := record.Actions[0], record.Actions[1], record.Actions[2], record.Actions[3]
	if write.Tool != "local_write" || write.Target != "hola.txt (create)" || write.Why != "create the greeting file" || write.Decision != "auto_approve" || write.Outcome != turnCompleted {
		t.Fatalf("write %+v", write)
	}
	if exec.Target != "cat hola.txt" || exec.Decision != "approve" || exec.Outcome != turnCompleted || exec.ExitCode == nil || *exec.ExitCode != 0 || exec.OutputTail != "hola" {
		t.Fatalf("exec %+v", exec)
	}
	if remember.Tool != string(HostRememberName) || remember.Outcome != turnCompleted || len(record.Memories) != 1 || record.Memories[0].About != "project:Lab" || record.Memories[0].Key != "axlr:remember:abc" {
		t.Fatalf("remember %+v memories %+v", remember, record.Memories)
	}
	if denied.Outcome != turnDenied || denied.Reason != "never delete the root" || denied.Decision != "deny" {
		t.Fatalf("denied %+v", denied)
	}
	if strings.Join(record.Files, ",") != "hola.txt" || len(record.Commands) != 1 || !strings.HasSuffix(record.Commands[0], "→ exit 0") {
		t.Fatalf("files %v commands %v", record.Files, record.Commands)
	}
	if !record.HasResults() || record.Cost.Requests != 2 {
		t.Fatalf("results %+v", record)
	}
	summary := record.Summary()
	for _, part := range []string{"Turn 1:", "1 file written (hola.txt)", "1 command run", "1 memory recorded", "1 call denied by the person"} {
		if !strings.Contains(summary, part) {
			t.Fatalf("summary %q lacks %q", summary, part)
		}
	}
	evidence := record.Evidence("MADE axlr-session-x step turn #1")
	if evidence[0] != "MADE axlr-session-x step turn #1" || len(evidence) != 5 || !strings.Contains(evidence[1], "why: create the greeting file") || !strings.Contains(evidence[2], "exit 0") || !strings.Contains(evidence[4], "deny, denied (never delete the root)") {
		t.Fatalf("evidence %q", evidence)
	}
	output := record.Output()
	if output["turn"] != float64(1) || output["answer_kind"] != "changed" || len(output["actions"].([]any)) != 4 {
		t.Fatalf("output %+v", output)
	}
}

// The model's reply to the memory reminder is bookkeeping: the record keeps
// the answer the person got (seen on 13 October 2026, when a record's answer
// was "I recorded nothing in memory").
func TestBuildTurnRecordKeepsTheAnswerBeforeTheReminder(t *testing.T) {
	s := recordedTurn(t)
	if err := s.BeginTurn(MemoryReminderPrefix+" record it", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("I recorded nothing in memory.")); err != nil {
		t.Fatal(err)
	}
	if record := BuildTurnRecord(s, 0, 1, "project:Lab", nil); record.Answer != "Hecho: hola.txt creado y comprobado." || len(record.Actions) != 4 {
		t.Fatalf("record %+v", record)
	}
}

func TestBuildTurnRecordOfAnAnswerFromReadingHasNoResults(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("¿qué hace main?", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", root.ToolCall{ID: "r1", Name: "local_read", Arguments: mustObject(t, `{"path":"main.go"}`)})); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("r1", domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"status":"completed","output":{}}`}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("Imprime hola. ¿Quieres que lo cambie?")); err != nil {
		t.Fatal(err)
	}
	record := BuildTurnRecord(s, 0, 2, "project:Lab", nil)
	if record.HasResults() || record.Reads != 1 || record.AnswerKind != "asked" || record.Cost != nil || !strings.Contains(record.Summary(), "no file, command or memory") {
		t.Fatalf("record %+v", record)
	}
}

func TestRedactHidesKeysAndTokens(t *testing.T) {
	text := "export OPENROUTER_API_KEY=sk-or-v1-abcdefghijklmnop; curl -H 'Authorization: Bearer abcdefghijkl' ghp_0123456789abcdef AKIAABCDEFGHIJKLMNOP"
	got := Redact(text)
	for _, secret := range []string{"sk-or-v1-abcdefghijklmnop", "Bearer abcdefghijkl", "ghp_0123456789abcdef", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%q still shows %q", got, secret)
		}
	}
	if Redact("go test ./...") != "go test ./..." {
		t.Fatal("plain text must stay")
	}
}

func TestClassifyOutcomeReadsEveryResultShape(t *testing.T) {
	for result, want := range map[string]string{
		``:                         turnUnknown,
		`tool call denied by user`: turnDenied,
		`not run: over the turn's tool-call budget of 32 calls: x`:                     turnOverBudget,
		`invalid tool invocation rejected: local_exec: give why`:                       turnRejected,
		`{"status":"completed","output":{"exit_code":1},"error":null}`:                 turnCompleted,
		`{"status":"failed","output":null,"error":{"code":"not_found","message":"x"}}`: turnError,
		`{"status":"completed","output":{"is_error":true}}`:                            turnError,
		`{"error":"KMP is not connected in this console"}`:                             turnError,
		`{"accepted":true}`: turnCompleted,
	} {
		if got, _ := classifyOutcome(result); got != want {
			t.Fatalf("%q: got %s, want %s", result, got, want)
		}
	}
	code, tail := execResult(`{"status":"completed","output":{"exit_code":2,"stdout":"","stderr":"boom: TOKEN=abcdef"}}`)
	if code == nil || *code != 2 || tail != "boom: [redacted]" {
		t.Fatalf("%v %q", code, tail)
	}
}

// LastRecord holds valid JSON within the domain's bound: cutting the JSON
// left it over the bound and unparseable, which froze the instance (14
// October 2026).
func TestBoundedJSONStaysValidAndWithinTheLimit(t *testing.T) {
	record := TurnRecord{Turn: 3, PromptIndex: 7, AnswerKind: "changed", Prompt: "p", Answer: "a"}
	for i := 0; i < 30; i++ {
		record.Actions = append(record.Actions, TurnAction{Tool: "local_exec", Target: "go test ./...", Why: strings.Repeat("why ", 100), OutputTail: strings.Repeat(`"x" `, 1024), CallID: "c"})
	}
	if len(record.JSON()) <= domain.MaxSessionRecordBytes {
		t.Fatal("the record should start over the limit")
	}
	for _, limit := range []int{domain.MaxSessionRecordBytes, 2000, 200} {
		text := record.BoundedJSON(limit)
		var decoded map[string]any
		if len(text) > limit || json.Unmarshal([]byte(text), &decoded) != nil || decoded["turn"] != float64(3) {
			t.Fatalf("limit %d: %d bytes %.80q", limit, len(text), text)
		}
	}
	if len(record.Actions[0].OutputTail) == 0 {
		t.Fatal("BoundedJSON changed the caller's actions")
	}
	small := TurnRecord{Turn: 1}
	if small.BoundedJSON(domain.MaxSessionRecordBytes) != small.JSON() {
		t.Fatal("a record within the limit is kept whole")
	}
}

// The tail of a long output starts on a character, not inside one.
func TestExecResultTailStartsOnACharacter(t *testing.T) {
	// An odd number of bytes after the é's puts the cut inside one.
	stdout := strings.Repeat("é", turnOutputTail)
	encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"exit_code": 0, "stdout": stdout + "x", "stderr": ""}})
	_, tail := execResult(string(encoded))
	if tail == "" || !utf8.ValidString(tail) || len(tail) > turnOutputTail {
		t.Fatalf("tail %d bytes, valid %v", len(tail), utf8.ValidString(tail))
	}
}
