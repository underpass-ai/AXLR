package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// policyTools offers local_read, local_exec, the MCP bridge and two MCP
// tools, as a real catalog would.
func policyTools(t *testing.T) []domain.AvailableTool {
	t.Helper()
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	tool := func(name string, id domain.ToolIdentity) domain.AvailableTool {
		return domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName(name), Description: root.Text(name), Parameters: schema}, Identity: id}
	}
	read, _ := domain.NewLocalToolIdentity("read")
	exec, _ := domain.NewLocalToolIdentity("exec")
	list, _ := domain.NewHostToolIdentity(domain.HostOperationTools)
	bridge, _ := domain.NewHostToolIdentity(domain.HostOperationCallTool)
	query, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "observability", ToolName: "query_logs"})
	memory, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"})
	return []domain.AvailableTool{tool("local_read", read), tool("local_exec", exec), tool(string(HostToolsName), list), tool(string(HostCallToolName), bridge), tool("query_logs", query), tool("kmp_write_memory", memory)}
}

func policySession(t *testing.T, policy domain.ExecutionPolicy, calls ...root.ToolCall) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("go", policyTools(t)); err != nil {
		t.Fatal(err)
	}
	if len(calls) > 0 {
		if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func argued(id root.ToolCallID, name root.ToolName, arguments string) root.ToolCall {
	args, _ := root.NewJSONObject([]byte(arguments))
	return root.ToolCall{ID: id, Name: name, Arguments: args}
}

// A call the policy forbids is refused before any card, with a reason the
// model reads, and the turn goes on: the read runs, the model answers.
func TestPolicyRefusesAForbiddenCallBeforeAnyCard(t *testing.T) {
	policy, _ := domain.NewExecutionPolicy(domain.PolicyObserve, []domain.PolicyRule{{Name: "local_read"}}, nil)
	s := policySession(t, policy, argued("e1", "local_exec", `{"program":"touch","args":["x"],"why":"asked"}`), argued("r1", "local_read", `{"path":"README.md"}`))
	store := &memoryStore{}
	executed := []string{}
	var offered []root.ToolName
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil }), Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		for _, tool := range req.Tools {
			offered = append(offered, tool.Name)
		}
		return assistant("done"), nil
	})}, Approval: approvalFunc(func(domain.ToolIdentity) bool { return false }), Tools: executionFunc(func(_ context.Context, id domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		executed = append(executed, id.LocalOperation)
		return domain.ToolOutcome{Content: "contents"}, nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || s.Status() != domain.StatusComplete {
		t.Fatalf("%v %s", err, s.Status())
	}
	if strings.Join(executed, ",") != "read" {
		t.Fatalf("executed %v", executed)
	}
	activity := s.Export().Activity
	if activity[0].Decision != domain.DecisionDeny || activity[0].Outcome == nil || !activity[0].Outcome.IsError || !strings.HasPrefix(string(activity[0].Outcome.Content), domain.ForbiddenOutcomePrefix+": local_exec is not in the execution's allow list") {
		t.Fatalf("%+v", activity[0])
	}
	result := domain.NewExecutionResult(s, nil)
	if result.Status != domain.ExecutionCompleted || result.Calls[0].Status != domain.ExecutionCallForbidden || result.Calls[1].Status != domain.ExecutionCallCompleted {
		t.Fatalf("%+v", result)
	}
	// The model was offered the read and the listing only.
	names := make([]string, 0, len(offered))
	for _, name := range offered {
		names = append(names, string(name))
	}
	if strings.Join(names, " ") != "axlr_tools local_read" {
		t.Fatalf("offered %v", names)
	}
}

// act-with-approval puts an allowed write under a decision even when the
// run approves everything; act-within-policy lets the run's approvals
// decide, and the destructive list keeps its card either way.
func TestPolicyLevelDecidesTheCard(t *testing.T) {
	always := approvalFunc(func(domain.ToolIdentity) bool { return true })
	for name, tc := range map[string]struct {
		level   domain.PolicyLevel
		call    root.ToolCall
		pending bool
	}{
		"act-with-approval asks":              {domain.PolicyActWithApproval, argued("e1", "local_exec", `{"program":"touch","args":["x"],"why":"asked"}`), true},
		"act-within-policy runs":              {domain.PolicyActWithinPolicy, argued("e1", "local_exec", `{"program":"touch","args":["x"],"why":"asked"}`), false},
		"act-within-policy keeps destructive": {domain.PolicyActWithinPolicy, argued("e1", "local_exec", `{"program":"git","args":["push","--force"],"why":"asked"}`), true},
	} {
		t.Run(name, func(t *testing.T) {
			policy, _ := domain.NewExecutionPolicy(tc.level, nil, nil)
			s := policySession(t, policy, tc.call)
			executed := 0
			u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil }), Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
				return assistant("done"), nil
			})}, Approval: always, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
				executed++
				return domain.ToolOutcome{Content: "ran"}, nil
			})}
			if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if (len(s.Pending()) == 1) != tc.pending || (executed == 0) != tc.pending {
				t.Fatalf("pending %d executed %d", len(s.Pending()), executed)
			}
		})
	}
}

// A decision taken under the policy cannot run what it forbids: approving
// the forbidden call refuses it, and the deny keeps its own wording.
func TestPolicyForbidsADecidedCall(t *testing.T) {
	policy, _ := domain.NewExecutionPolicy(domain.PolicyObserve, nil, nil)
	for _, decision := range []domain.ToolDecision{domain.DecisionApprove, domain.DecisionDeny} {
		s := policySession(t, policy, argued("e1", "local_exec", `{"program":"touch","args":["x"],"why":"asked"}`))
		executed := 0
		u := ResolveToolUseCase{Store: &memoryStore{}, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil }), Continue: ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			return assistant("done"), nil
		})}, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
			executed++
			return domain.ToolOutcome{Content: "ran"}, nil
		})}
		if err := u.Execute(context.Background(), &s, "e1", decision, ignoreEvent); err != nil || executed != 0 {
			t.Fatalf("%s: %v executed %d", decision, err, executed)
		}
		if outcome := s.Export().Activity[0].Outcome; outcome == nil || !strings.HasPrefix(string(outcome.Content), domain.ForbiddenOutcomePrefix) {
			t.Fatalf("%s: %+v", decision, outcome)
		}
	}
}

// The policy also filters what axlr_tools lists and what axlr_call_tool
// may reach: an MCP tool the allow list leaves out is not offered.
func TestPolicyFiltersTheToolsOffered(t *testing.T) {
	policy, _ := domain.NewExecutionPolicy(domain.PolicyObserve, []domain.PolicyRule{{Name: "observability/*"}}, nil)
	s := policySession(t, policy)
	var direct []string
	for _, tool := range SessionTools(s, s.ToolSnapshot()) {
		direct = append(direct, string(tool.Name))
	}
	if strings.Join(direct, " ") != "axlr_call_tool axlr_tools" {
		t.Fatalf("direct %v", direct)
	}
	var discovered []string
	for _, tool := range discoverySnapshot(s) {
		discovered = append(discovered, string(tool.Definition.Name))
	}
	if strings.Join(discovered, " ") != "axlr_tools axlr_call_tool query_logs" {
		t.Fatalf("discovered %v", discovered)
	}
	// The bridge resolves to the MCP tool, which the policy judges: the
	// memory write is forbidden, the query allowed.
	for name, want := range map[string]bool{"query_logs": false, "kmp_write_memory": true} {
		tool, args, known, err := ResolveToolCall(s.ToolSnapshot(), argued("c1", HostCallToolName, `{"name":"`+name+`","arguments":{},"why":"asked"}`))
		if !known || err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, forbidden := policyRefusal(s, tool, args); forbidden != want {
			t.Fatalf("%s forbidden=%v", name, forbidden)
		}
	}
}
