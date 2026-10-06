package application

import (
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Outcome contents as the console records them for the model.
const (
	outcomeInternal   = `{"protocol_version":1,"request_id":"tui-tool","tool":"edit","status":"failed","output":null,"error":{"code":"internal_error","message":"edit: unexpected nil in replace"}}`
	outcomeFilesystem = `{"status":"failed","error":{"code":"filesystem_error","message":"rename: input/output error"}}`
	outcomeExitOne    = `{"status":"completed","output":{"exit_code":1,"stdout":"","stderr":"FAIL TestParse"}}`
	outcomeExitZero   = `{"status":"completed","output":{"exit_code":0,"stdout":"ok","stderr":""}}`
	outcomePermission = `{"status":"failed","error":{"code":"permission_denied","message":"open secret.txt: permission denied"}}`
	outcomeNotFound   = `{"status":"failed","error":{"code":"start_failed","message":"program \"gox\" not found in configured PATH"}}`
	outcomeRejected   = `{"status":"rejected","error":{"code":"invalid_path","message":"path escapes the workspace"}}`
	outcomeTimedOut   = `{"status":"timed_out","error":{"code":"timeout","message":"exec exceeded 300000ms"}}`
	outcomeHostArgs   = `{"error":"limit must be an integer between 1 and 20"}`
	outcomeHostBroken = `{"error":"unreadable page for message 3"}`
	outcomeHostOK     = `{"content":"","returned_bytes":0,"next_offset_bytes":0}`
	outcomeRunErr     = "tool execution failed; effect unknown: runtime error: index out of range [3] with length 2"
	outcomeRunErrExt  = "tool execution failed; effect unknown: openrouter: 401 unauthorized"
	outcomePlugin     = `{"status":"completed","output":{"content":[{"type":"text","text":"boom"}],"is_error":true}}`
	outcomeDenied     = "tool call denied by user"
)

type activity struct {
	id, tool, args, outcome string
	deny                    bool
}

// repairTools is a session snapshot with local tools, the host tools and one
// plugin tool, as a real console session has.
func repairTools(t *testing.T) []domain.AvailableTool {
	t.Helper()
	tools := append(turnTools(), HostTools()...)
	schema := mustObject(t, `{"type":"object"}`)
	for _, op := range []string{"edit", "exec", "write"} {
		id, err := domain.NewLocalToolIdentity(op)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName("local_" + op), Parameters: schema}})
	}
	return append(tools, hostPlugin(t, "kmp_wake", "kmp", "kmp_wake"))
}

// parentSession is a session that made the given calls and saw their outcomes.
func parentSession(t *testing.T, activities ...activity) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	var calls []root.ToolCall
	for _, a := range activities {
		args := a.args
		if args == "" {
			args = `{}`
		}
		calls = append(calls, root.ToolCall{ID: root.ToolCallID(a.id), Name: root.ToolName(a.tool), Arguments: mustObject(t, args)})
	}
	if len(calls) > 0 {
		if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
			t.Fatal(err)
		}
		for _, a := range activities {
			decision := domain.DecisionApprove
			if a.deny {
				decision = domain.DecisionDeny
			}
			if err := s.RecordToolOutcome(root.ToolCallID(a.id), decision, domain.ToolOutcome{Content: root.Text(a.outcome), IsError: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.CompleteAssistant(assistant("I noticed the failure.")); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClassifyCallSeparatesDefectsOfAXLRFromEverythingElse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		activity activity
		class    string
		refusal  string
	}{
		{"internal", activity{"c", "local_edit", "", outcomeInternal, false}, classRuntimeFailure, ""},
		{"filesystem", activity{"c", "local_write", "", outcomeFilesystem, false}, classRuntimeFailure, ""},
		{"host failure", activity{"c", "axlr_history", `{"message_index":3}`, outcomeHostBroken, false}, classHostError, ""},
		{"host wrong result", activity{"c", "axlr_history", `{"message_index":3}`, outcomeHostOK, false}, classWrongResult, ""},
		{"local wrong result", activity{"c", "local_exec", "", outcomeExitZero, false}, classWrongResult, ""},
		{"effect unknown", activity{"c", "local_edit", "", outcomeRunErr, false}, classRuntimeFailure, ""},
		{"unreadable", activity{"c", "local_edit", "", "garbage", false}, classRuntimeFailure, ""},
		{"project exit", activity{"c", "local_exec", "", outcomeExitOne, false}, "", "exited 1"},
		{"permission", activity{"c", "read", "", outcomePermission, false}, "", "permission denied"},
		{"program missing", activity{"c", "local_exec", "", outcomeNotFound, false}, "", "the workspace, the program"},
		{"rejected", activity{"c", "read", "", outcomeRejected, false}, "", "rejected by AXLR"},
		{"timed out", activity{"c", "local_exec", "", outcomeTimedOut, false}, "", "not proof of a defect"},
		{"host arguments", activity{"c", "axlr_tools", `{"limit":0}`, outcomeHostArgs, false}, "", "refused for its arguments"},
		{"effect unknown external", activity{"c", "local_edit", "", outcomeRunErrExt, false}, "", "external cause"},
		{"plugin", activity{"c", "kmp_wake", `{"about":"x"}`, outcomePlugin, false}, "", "MCP plugin tool"},
		{"denied", activity{"c", "local_edit", "", outcomeDenied, true}, "", "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := parentSession(t, tc.activity)
			state := s.Export()
			cited, refusal := classifyCall(state.ToolSnapshot, state.Activity[0])
			if tc.refusal == "" && (refusal != "" || cited.Class != tc.class) {
				t.Fatalf("class %q refusal %q", cited.Class, refusal)
			}
			if tc.refusal != "" && !strings.Contains(refusal, tc.refusal) {
				t.Fatalf("refusal %q, want %q", refusal, tc.refusal)
			}
		})
	}
	s := parentSession(t)
	if _, refusal := classifyCall(s.Export().ToolSnapshot, domain.PendingTool{Call: root.ToolCall{ID: "x", Name: "unknown"}, Outcome: &domain.ToolOutcome{Content: "?"}}); !strings.Contains(refusal, "does not know") {
		t.Fatalf("unknown tool: %q", refusal)
	}
	if _, refusal := classifyCall(s.Export().ToolSnapshot, domain.PendingTool{Call: root.ToolCall{ID: "x", Name: "local_edit"}}); !strings.Contains(refusal, "no outcome") {
		t.Fatalf("pending call: %q", refusal)
	}
}

func TestExternalCauseMatchesWholeWordsOnly(t *testing.T) {
	if cause := externalCause("local_edit returns internal_error on every call"); cause != "" {
		t.Fatalf("false positive %q", cause)
	}
	if cause := externalCause("the tools list is unreadable"); cause != "" {
		t.Fatalf("tls inside tools: %q", cause)
	}
	for text, want := range map[string]string{"OpenRouter answered 429": "openrouter", "gh says not logged in": "not logged in", "the API key expired": "api key", "ECONNREFUSED on port 8080": "econnrefused"} {
		if cause := externalCause("note", text); cause != want {
			t.Fatalf("%q: %q, want %q", text, cause, want)
		}
	}
}

func TestRecursNeedsTheSameFailureTwiceOrTwoWrongResults(t *testing.T) {
	once := parentSession(t, activity{"c1", "local_edit", "", outcomeInternal, false}, activity{"c2", "local_exec", "", outcomeExitOne, false})
	failures, refusal := citedFailures(once, repairRequest{ToolCalls: []string{"c1"}})
	if refusal != "" || recurs(once, failures) {
		t.Fatalf("an isolated failure recurred: %q %v", refusal, failures)
	}
	twice := parentSession(t, activity{"c1", "local_edit", "", outcomeInternal, false}, activity{"c2", "local_edit", "", outcomeInternal, false})
	failures, _ = citedFailures(twice, repairRequest{ToolCalls: []string{"c2"}})
	if !recurs(twice, failures) {
		t.Fatal("the uncited repeat must count")
	}
	wrong := parentSession(t, activity{"c1", "axlr_history", `{"message_index":1}`, outcomeHostOK, false}, activity{"c2", "axlr_history", `{"message_index":1}`, outcomeHostOK, false})
	failures, _ = citedFailures(wrong, repairRequest{ToolCalls: []string{"c1"}})
	if recurs(wrong, failures) {
		t.Fatal("one wrong result is not evidence")
	}
	failures, _ = citedFailures(wrong, repairRequest{ToolCalls: []string{"c1", "c2"}})
	if !recurs(wrong, failures) {
		t.Fatal("two cited wrong results are evidence")
	}
	if _, refusal := citedFailures(wrong, repairRequest{ToolCalls: []string{"missing"}}); !strings.Contains(refusal, "not in this session") {
		t.Fatalf("unknown call: %q", refusal)
	}
}

func TestRepairSignatureIgnoresWordingAndOrder(t *testing.T) {
	a := repairSignature("o/r", []citedFailure{{Tool: "local_edit", Class: classRuntimeFailure}, {Tool: "axlr_history", Class: classHostError}})
	b := repairSignature("o/r", []citedFailure{{Tool: "axlr_history", Class: classHostError, Detail: "other words"}, {Tool: "local_edit", Class: classRuntimeFailure}})
	c := repairSignature("o/other", []citedFailure{{Tool: "local_edit", Class: classRuntimeFailure}})
	if a != b || a == c || len(a) != 16 {
		t.Fatalf("signatures %s %s %s", a, b, c)
	}
}

func TestDecodeRepairRequestBoundsEveryField(t *testing.T) {
	valid := `{"description":"local_edit fails with internal_error on every replace","expected":"the file is edited","observed":"internal_error: unexpected nil","evidence":["two identical failures"],"tool_calls":["c1","c2"]}`
	if _, err := decodeRepairRequest(mustObject(t, valid)); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]string{
		"short":       `{"description":"it broke","expected":"e","observed":"o","evidence":["x"],"tool_calls":["c1"]}`,
		"no expected": `{"description":"local_edit fails with internal_error on every replace","observed":"o","evidence":["x"],"tool_calls":["c1"]}`,
		"no evidence": `{"description":"local_edit fails with internal_error on every replace","expected":"e","observed":"o","evidence":[],"tool_calls":["c1"]}`,
		"no calls":    `{"description":"local_edit fails with internal_error on every replace","expected":"e","observed":"o","evidence":["x"],"tool_calls":[]}`,
		"dup calls":   `{"description":"local_edit fails with internal_error on every replace","expected":"e","observed":"o","evidence":["x"],"tool_calls":["c1","c1"]}`,
		"blank item":  `{"description":"local_edit fails with internal_error on every replace","expected":"e","observed":"o","evidence":[" "],"tool_calls":["c1"]}`,
		"unknown":     `{"description":"local_edit fails with internal_error on every replace","expected":"e","observed":"o","evidence":["x"],"tool_calls":["c1"],"severity":"high"}`,
	} {
		if _, err := decodeRepairRequest(mustObject(t, args)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestRepairBriefCarriesTheCitedOutcomesAndRepairSlugNamesIt(t *testing.T) {
	s := parentSession(t, activity{"c1", "local_edit", `{"path":"a.go"}`, outcomeInternal, false})
	request := repairRequest{Description: "local_edit fails with internal_error on every replace", Expected: "edited", Observed: "nil", Evidence: []string{"seen twice"}, ToolCalls: []string{"c1"}}
	failures, _ := citedFailures(s, request)
	brief := repairBrief(s, request, failures, "0.3.0")
	for _, want := range []string{"Expected: edited", "- seen twice", "c1 local_edit", "unexpected nil in replace", "build 0.3.0", "Reproduce the defect"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief lacks %q:\n%s", want, brief)
		}
	}
	now := time.Date(2026, 10, 5, 1, 2, 0, 0, time.UTC)
	if got := RepairSlug("go test ./... fails: TestWordCount expects 2, gets 3", now); got != "20261005-0102-go-test-fails-testwordcount-expects-2" {
		t.Fatalf("slug %q", got)
	}
	if got := RepairSlug("¿¡!?", now); got != "20261005-0102-failure" {
		t.Fatalf("empty words %q", got)
	}
}
