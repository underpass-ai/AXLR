package application

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
)

type skillReadFunc func(context.Context, string, string, string, int, int) (SkillPage, error)

func (f skillReadFunc) ReadSkill(ctx context.Context, plugin, skill, path string, offset, limit int) (SkillPage, error) {
	return f(ctx, plugin, skill, path, offset, limit)
}

func TestAutomaticSkillReadUsesInstalledCatalog(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("read installed skill", append(turnTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", root.ToolCall{ID: "skill", Name: HostSkillName, Arguments: hostJSON(t, `{"plugin":"sample","skill":"example"}`)})); err != nil {
		t.Fatal(err)
	}
	reads := 0
	reader := skillReadFunc(func(_ context.Context, plugin, skill, path string, offset, limit int) (SkillPage, error) {
		reads++
		if plugin != "sample" || skill != "example" || path != "SKILL.md" || offset != 0 || limit != 4096 {
			t.Fatalf("wrong skill read: %s %s %s %d %d", plugin, skill, path, offset, limit)
		}
		return SkillPage{Plugin: plugin, Skill: skill, Text: "instructions", TotalBytes: 12, NextOffsetBytes: 12}, nil
	})
	store := &memoryStore{}
	models := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("done"), nil
	})
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Models: models, PluginSkills: reader}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	if reads != 1 || len(activity) != 1 || activity[0].Decision != domain.DecisionAutoApprove || activity[0].Outcome.IsError || !strings.Contains(string(activity[0].Outcome.Content), "instructions") || s.Status() != domain.StatusComplete {
		t.Fatalf("skill auto read: reads=%d activity=%+v state=%s", reads, activity, s.Status())
	}
}

type approvalFunc func(domain.ToolIdentity) bool

func (f approvalFunc) AutoApproves(id domain.ToolIdentity) bool { return f(id) }

func TestAutomaticApprovalPersistsEachEffectAndStopsAtManualCall(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	tools := turnTools()
	ref := root.PluginRef{PluginID: "kmp", ToolName: "kmp_wake"}
	identity, _ := domain.NewPluginToolIdentity(ref)
	tools = append(tools, domain.AvailableTool{Definition: root.ToolDefinition{Name: "memory", Description: "memory", Parameters: turnTools()[0].Definition.Parameters}, Identity: identity})
	if err := s.BeginTurn("go", tools); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", call("a", "memory"), call("b", "memory"), call("c", "read"))); err != nil {
		t.Fatal(err)
	}
	executed := 0
	snapshots := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil })}, Approval: approvalFunc(func(id domain.ToolIdentity) bool { return id == identity }), Tools: executionFunc(func(_ context.Context, id domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		executed++
		st := store.states[len(store.states)-1]
		if id != identity || st.Status != domain.StatusInterrupted || st.Activity[executed-1].Decision != domain.DecisionAutoApprove || !st.Activity[executed-1].Outcome.Uncertain {
			t.Fatal("missing durable auto effect checkpoint")
		}
		return domain.ToolOutcome{Content: "memory result"}, nil
	})}
	err := u.Execute(context.Background(), &s, func(e Event) error {
		if e.Kind == EventSession {
			snapshots++
			if e.Snapshot == nil {
				t.Fatal("missing snapshot")
			}
		}
		return nil
	})
	if err != nil || executed != 2 || snapshots != 2 || len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "c" {
		t.Fatalf("auto/manual boundary: %v %d %+v", err, executed, s.Export())
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticApprovalUncertainCallCannotReplay(t *testing.T) {
	s := queued(t, call("a", "read"), call("b", "read"))
	store := &memoryStore{}
	executions := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store}, Approval: approvalFunc(func(domain.ToolIdentity) bool { return true }), Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executions++
		return domain.ToolOutcome{}, errors.New("transport lost")
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || s.Status() != domain.StatusInterrupted {
		t.Fatal("uncertain auto call continued")
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || executions != 1 {
		t.Fatal("uncertain auto call replayed")
	}
	restored, err := domain.RestoreSession(store.states[len(store.states)-1])
	if err != nil {
		t.Fatal(err)
	}
	if restored.Export().Activity[0].Decision != domain.DecisionAutoApprove {
		t.Fatal("lost auto decision")
	}
}

func TestAutomaticDecisionCannotBypassPublicHumanResolver(t *testing.T) {
	s := queued(t, call("a", "read"))
	u := ResolveToolUseCase{Store: &memoryStore{}}
	if u.Execute(context.Background(), &s, "a", domain.DecisionAutoApprove, nil) == nil {
		t.Fatal("public bypass")
	}
	if u.resolveOne(context.Background(), &s, "a", domain.DecisionAutoApprove, ignoreEvent) == nil {
		t.Fatal("policy bypass")
	}
}

func TestModelContextReflectsRegisteredCapabilitiesAndPreservesHistory(t *testing.T) {
	s := turnSession(t)
	tools := turnTools()
	for _, id := range []root.PluginID{"made", "kmp"} {
		identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: id, ToolName: "guide"})
		tools = append(tools, domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{Name: root.ToolName(string(id) + "_guide"), Description: "guide", Parameters: turnTools()[0].Definition.Parameters}})
	}
	if err := s.BeginTurn("hello", tools); err != nil {
		t.Fatal(err)
	}
	messages := modelMessages(&s)
	for _, text := range []string{"MADE", "KMP", "specific extended topic", "made_guide", "kmp_guide"} {
		if !strings.Contains(string(messages[0].Content), text) {
			t.Fatalf("missing %s", text)
		}
	}
	if len(s.Messages()) != 1 || s.Messages()[0].Role != root.RoleUser || messages[1].Content != "hello" {
		t.Fatal("changed persisted history")
	}
}
