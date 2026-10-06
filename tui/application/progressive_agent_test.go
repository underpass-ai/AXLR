package application

import (
	"context"
	"errors"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestProgressiveAgentDiscoveryDoesNotAuthorizeManualPlugin(t *testing.T) {
	s := turnSession(t)
	kmp := hostPlugin(t, "memory", "kmp", "kmp_ask")
	made := hostPlugin(t, "ceremony", "made", "run")
	tools := append(turnTools(), HostTools()...)
	tools = append(tools, kmp, made)
	if err := s.BeginTurn("use memory and then a ceremony", tools); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	calls, executions, phases := 0, 0, 0
	validation := argumentValidationFunc(func(def root.ToolDefinition, args root.JSONValue) error {
		if (def.Name != "memory" && def.Name != "ceremony") || string(args.Bytes()) != `{"about":"project:fixture"}` {
			t.Fatal("validated wrapper instead of real arguments")
		}
		return nil
	})
	models := streamFunc(func(ctx context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if len(req.Tools) != 8 || requestHasTool(req, "memory") || requestHasTool(req, "ceremony") {
			t.Fatal("full plugin schemas leaked into fixed model catalog")
		}
		NotifyProviderActivity(ctx, domain.ProviderReasoning)
		calls++
		switch calls {
		case 1:
			return assistant("", root.ToolCall{ID: "discover", Name: HostToolsName, Arguments: hostJSON(t, `{"name":"memory"}`)}), nil
		case 2:
			return assistant("", root.ToolCall{ID: "remember", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"memory","arguments":{"about":"project:fixture"}}`)}), nil
		case 3:
			return assistant("", root.ToolCall{ID: "ceremony", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"ceremony","arguments":{"about":"project:fixture"}}`)}), nil
		default:
			return assistant("finished"), nil
		}
	})
	runner := executionFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		executions++
		if string(args.Bytes()) != `{"about":"project:fixture"}` {
			t.Fatal("wrapper arguments reached plugin")
		}
		last := store.states[len(store.states)-1]
		if last.Status != domain.StatusInterrupted || !last.Activity[len(last.Activity)-1].Outcome.Uncertain {
			t.Fatal("effect lacks durable checkpoint")
		}
		if executions == 1 && id != kmp.Identity {
			t.Fatal("discovery autoapproval bypassed plugin policy")
		}
		if executions == 2 && id != made.Identity {
			t.Fatal("human approval lost real plugin identity")
		}
		return domain.ToolOutcome{Content: "stored evidence"}, nil
	})
	continuation := ContinueTurnUseCase{Store: store, Models: models, Validation: validation}
	policy := approvalFunc(func(id domain.ToolIdentity) bool { return id == kmp.Identity })
	emit := func(e Event) error {
		if e.Kind == EventProviderActivity {
			phases++
		}
		return nil
	}
	if err := (AgentTurnUseCase{Continue: continuation, Approval: policy, Tools: runner}).Execute(context.Background(), &s, emit); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || executions != 1 || phases != 3 || s.Status() != domain.StatusApproval || len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "ceremony" {
		t.Fatalf("wrong manual boundary calls=%d effects=%d state=%s", calls, executions, s.Status())
	}
	if s.Export().Activity[0].Decision != domain.DecisionAutoApprove || s.Export().Activity[1].Decision != domain.DecisionAutoApprove {
		t.Fatal("discovery or configured memory did not autoapprove")
	}
	resolver := ResolveToolUseCase{Store: store, Tools: runner, Approval: policy, Continue: continuation, Validation: validation}
	if err := resolver.Execute(context.Background(), &s, "ceremony", domain.DecisionApprove, emit); err != nil {
		t.Fatal(err)
	}
	if calls != 4 || executions != 2 || s.Status() != domain.StatusComplete {
		t.Fatal("human ceremony approval did not continue")
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatal(err)
	}
}

func TestProgressiveAgentRejectsInvalidSchemaBeforeAnyEffect(t *testing.T) {
	for _, validator := range []ToolArgumentValidationPort{nil, argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return errors.New("required about field is missing") })} {
		s := turnSession(t)
		plugin := hostPlugin(t, "memory", "kmp", "kmp_ask")
		tools := append(HostTools(), plugin)
		_ = s.BeginTurn("use memory", tools)
		_ = s.CompleteAssistant(assistant("", root.ToolCall{ID: "bad", Name: HostCallToolName, Arguments: hostJSON(t, `{"name":"memory","arguments":{}}`)}))
		store := &memoryStore{}
		continuation := ContinueTurnUseCase{Store: store, Validation: validator, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			return assistant("repair required"), nil
		})}
		if err := (AgentTurnUseCase{Continue: continuation, Approval: approvalFunc(func(domain.ToolIdentity) bool { return true }), Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
			t.Fatal("invalid plugin arguments executed")
			return domain.ToolOutcome{}, nil
		})}).Execute(context.Background(), &s, ignoreEvent); err != nil {
			t.Fatal(err)
		}
		a := s.Export().Activity[0]
		if a.Decision != domain.DecisionDeny || a.Outcome == nil || !a.Outcome.IsError || a.Outcome.Uncertain {
			t.Fatal("argument rejection became an uncertain effect")
		}
		if _, err := domain.RestoreSession(s.Export()); err != nil {
			t.Fatal(err)
		}
	}
}
