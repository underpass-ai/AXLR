package diagnostics

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// A finished call is one line: INFO when it succeeded, with the plugin's
// tool it reached; WARN when it failed, with what it answered on one line,
// whole up to the bound and redacted like every entry.
func TestToolCallLogWritesOneLinePerFinishedCall(t *testing.T) {
	log, err := OpenAppLog(t.TempDir(), "sk-or-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.now = func() time.Time { return time.Date(2026, 11, 12, 18, 2, 36, 0, time.UTC) }
	adapter := ToolCallLog{Log: log}
	adapter.ToolCallFinished(application.ToolCallRecord{Session: "a062c091657295be0dae7c3e90ea51a8", Call: "call-1", Tool: "mcp_0123456789abcdef", PluginTool: "kmp/kmp_ask", Decision: "auto_approve", Duration: 41 * time.Millisecond})
	refusal := `{"call_id":"call-2","error":"the execution's policy keeps what it writes to its own abouts and its team's (project:arte.arte-orquestador, project:arte, ws:a062c091): project:arte.arte-compositor is a teammate's or another's, which it may read and not write"}`
	adapter.ToolCallFinished(application.ToolCallRecord{Session: "a062c091657295be0dae7c3e90ea51a8", Call: "call-2", Tool: "axlr_remember", Decision: "auto_approve", Failed: true, Error: refusal, Duration: 11 * time.Millisecond})
	adapter.ToolCallFinished(application.ToolCallRecord{Session: "s", Call: "call-3", Tool: "local_exec", Failed: true, Error: "key sk-or-secret-value\nsecond line"})
	pid := fmt.Sprint(os.Getpid())
	lines := strings.Split(strings.TrimSpace(readLog(t, log.Path())), "\n")
	want := []string{
		"2026-11-12T18:02:36.000Z INFO  [" + pid + "] tool call: session a062c091657295be0dae7c3e90ea51a8, call call-1, tool mcp_0123456789abcdef (kmp/kmp_ask), decision auto_approve, ok in 41ms",
		"2026-11-12T18:02:36.000Z WARN  [" + pid + "] tool call: session a062c091657295be0dae7c3e90ea51a8, call call-2, tool axlr_remember, decision auto_approve, failed in 11ms: " + refusal,
		"2026-11-12T18:02:36.000Z WARN  [" + pid + "] tool call: session s, call call-3, tool local_exec, decision none, failed in 0s: key [redacted] | second line",
	}
	if len(lines) != len(want) {
		t.Fatalf("log = %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d\n got %q\nwant %q", i, lines[i], want[i])
		}
	}
}

// A long failure is bounded so its whole entry fits the log's line, never
// cut by the log with its marker; a call without a log records nothing.
func TestToolCallLogBoundsAFailureToTheLogsLine(t *testing.T) {
	log, err := OpenAppLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	ToolCallLog{Log: log}.ToolCallFinished(application.ToolCallRecord{Session: "0123456789abcdef0123456789abcdef", Call: "call_0123456789abcdef", Tool: "axlr_call_tool", PluginTool: "made/made_start_published_ceremony", Decision: "auto_approve", Failed: true, Error: strings.Repeat("é", application.MaxToolCallErrorBytes) + " the end"})
	text := strings.TrimSpace(readLog(t, log.Path()))
	if strings.Contains(text, "[cut]") || !strings.Contains(text, " … ") || !strings.HasSuffix(text, "é the end") || !strings.Contains(text, "WARN") {
		t.Fatalf("log = %q", text)
	}
	ToolCallLog{}.ToolCallFinished(application.ToolCallRecord{Failed: true, Error: "nowhere"})
}
