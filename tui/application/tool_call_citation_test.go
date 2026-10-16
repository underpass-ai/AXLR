package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 16 October 2026 Haiku 5.5 cited "axlr_forge_tool#1 (hello_probe, first
// attempt)" in axlr_request_repair: Claude models never see the IDs of their
// own calls, so every result has to name its call.
func TestEveryToolResultNamesTheCallTheModelCanCite(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("go", append(turnTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	history := root.ToolCall{ID: "toolu_h1", Name: HostHistoryName, Arguments: mustObject(t, `{"message_index":0}`)}
	if err := s.CompleteAssistant(assistant("", call("toolu_l1", "read"), history)); err != nil {
		t.Fatal(err)
	}
	var seen root.ToolCallID
	store := &memoryStore{}
	done := ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("done"), nil
	})}
	u := ResolveToolUseCase{Store: store, Continue: done, Tools: executionFunc(func(ctx context.Context, _ domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		seen, _ = ToolCallOf(ctx)
		return domain.ToolOutcome{Content: `{"protocol_version":1,"request_id":"toolu_l1","tool":"exec","status":"completed","output":{"exit_code":0}}`}, nil
	})}
	// The host call runs on its own once the person approves the first.
	if err := u.Execute(context.Background(), &s, "toolu_l1", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if seen != "toolu_l1" {
		t.Fatalf("the runtime ran the call as %q, not as its call ID", seen)
	}
	activity := s.Export().Activity
	if activity[1].Outcome == nil {
		t.Fatal("the host call did not run")
	}
	if content := string(activity[1].Outcome.Content); !strings.HasPrefix(content, `{"call_id":"toolu_h1",`) {
		t.Fatalf("host result does not name its call: %s", content)
	}
	if content := string(activity[0].Outcome.Content); strings.Contains(content, "call_id") {
		t.Fatalf("a runtime envelope already names its call in request_id: %s", content)
	}
}

func TestTheProjectionKeepsTheCallIDOfARuntimeEnvelope(t *testing.T) {
	envelope := `{"protocol_version":1,"request_id":"toolu_l1","tool":"read","status":"completed","output":{"content":"hello"},"error":null}`
	whole, err := projectToolContentIn(envelope, 3, 8192, excerptHistorical, "local_read")
	if err != nil || !strings.Contains(whole, `"request_id":"toolu_l1"`) {
		t.Fatalf("whole result lost its call: %s %v", whole, err)
	}
	large := `{"protocol_version":1,"request_id":"toolu_l2","tool":"read","status":"completed","output":{"content":"` + strings.Repeat("x", 20000) + `"},"error":null}`
	excerpt, err := projectToolContentIn(large, 3, 4096, excerptHistorical, "local_exec")
	if err != nil || !strings.Contains(excerpt, `"request_id":"toolu_l2"`) {
		t.Fatalf("excerpt lost its call: %.300s %v", excerpt, err)
	}
	host := `{"call_id":"toolu_h1","content":"` + strings.Repeat("y", 20000) + `"}`
	excerpt, err = projectToolContentIn(host, 4, 4096, excerptHistorical, "axlr_logs")
	if err != nil || !strings.Contains(excerpt, `"call_id":"toolu_h1"`) {
		t.Fatalf("host excerpt lost its call: %.300s %v", excerpt, err)
	}
	// Results recorded before calls were named keep their text, so a
	// resumed session keeps the prefix the provider cached.
	old := `{"protocol_version":1,"request_id":"tui-tool","tool":"read","status":"completed","output":{"content":"hello"},"error":null}`
	if whole, _ := projectToolContentIn(old, 3, 8192, excerptHistorical, "local_read"); strings.Contains(whole, "tui-tool") || strings.Contains(whole, "call_id") {
		t.Fatalf("an old result changed its projection: %s", whole)
	}
}

func TestARepairCitingALabelIsToldTheCallsItCanCite(t *testing.T) {
	s := parentSession(t, activity{"toolu_a", "local_edit", "", outcomeInternal, false}, activity{"toolu_b", "local_edit", "", outcomeInternal, false}, activity{"toolu_c", "kmp_wake", "", outcomePlugin, false})
	label := "local_edit#1 (first attempt)"
	_, refusal := citedFailures(s, repairRequest{ToolCalls: []string{label}})
	if !strings.Contains(refusal, "not in this session") || !strings.Contains(refusal, "toolu_a (local_edit") || !strings.Contains(refusal, "toolu_b (local_edit") {
		t.Fatalf("repair refusal names no citable call: %q", refusal)
	}
	if strings.Contains(refusal, "toolu_c") {
		t.Fatalf("a plugin call is not AXLR's to repair: %q", refusal)
	}
	_, refusal = citedFriction(s, repairRequest{ToolCalls: []string{"toolu_a", label}})
	if !strings.Contains(refusal, "toolu_a (local_edit") || !strings.Contains(refusal, "toolu_b (local_edit") {
		t.Fatalf("improvement refusal names no citable call: %q", refusal)
	}
}
