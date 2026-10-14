package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 claude-haiku-5.5 sent local_exec timeout_ms 600000; the
// person approved the card and the runtime refused the call in 0 ms. Local
// arguments are now checked against their schema before any card.
func TestLocalArgumentsOutsideTheSchemaAreRefusedBeforeTheCard(t *testing.T) {
	s := turnSession(t)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"timeout_ms":{"type":"integer","maximum":300000}}}`))
	id, _ := domain.NewLocalToolIdentity("exec")
	if err := s.BeginTurn("run the tests", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_exec", Parameters: schema}, Identity: id}}); err != nil {
		t.Fatal(err)
	}
	validated := 0
	validation := argumentValidationFunc(func(tool root.ToolDefinition, args root.JSONValue) error {
		validated++
		if strings.Contains(string(args.Bytes()), "600000") {
			return errors.New("validating /properties/timeout_ms: maximum: 600000 is greater than 300000")
		}
		return nil
	})
	requests := 0
	models := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		requests++
		if requests == 1 {
			return assistant("", root.ToolCall{ID: "slow", Name: "local_exec", Arguments: hostJSON(t, `{"program":"go","args":["test","./..."],"timeout_ms":600000}`)}), nil
		}
		return assistant("", root.ToolCall{ID: "fits", Name: "local_exec", Arguments: hostJSON(t, `{"program":"go","args":["test","./..."],"timeout_ms":120000}`)}), nil
	})
	executed := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Models: models, Validation: validation}, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executed++
		return domain.ToolOutcome{Content: "ok"}, nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	if executed != 0 || requests != 2 || len(activity) != 2 || validated < 2 {
		t.Fatalf("executed=%d requests=%d validated=%d activity=%+v", executed, requests, validated, activity)
	}
	refused := activity[0]
	if refused.Decision != domain.DecisionDeny || refused.Outcome == nil || !strings.Contains(string(refused.Outcome.Content), "greater than 300000") || !strings.Contains(string(refused.Outcome.Content), "local_exec") {
		t.Fatalf("over-limit call not refused before the card: %+v", refused)
	}
	// The corrected call waits for the person's decision as before.
	if pending := s.Pending(); s.Status() != domain.StatusApproval || len(pending) != 1 || pending[0].Call.ID != "fits" {
		t.Fatalf("corrected call: %s %+v", s.Status(), pending)
	}
}

// The runtime's checks that read no file run before the card too, with its
// exact messages. On 14 October 2026 a local_write replace without
// expected_sha256 raised a card, the person approved it, the runtime
// refused it, and the person approved the same content again.
func TestLocalCallsTheRuntimeWouldRefuseAreRefusedBeforeTheCard(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for name, test := range map[string]struct {
		operation, arguments, want string
	}{
		"replace without digest": {"write", `{"path":"calc/calc.go","content":"x","mode":"replace"}`, "local_write: replace requires expected_sha256: pass the content_sha256 of a local_read of calc/calc.go"},
		"create with digest":     {"write", `{"path":"a.go","content":"x","mode":"create","expected_sha256":"` + digest + `"}`, "local_write: create does not accept expected_sha256"},
		"bad digest":             {"write", `{"path":"a.go","content":"x","mode":"replace","expected_sha256":"abc"}`, "local_write: invalid SHA-256 digest"},
		"outside the workspace":  {"read", `{"path":"../secret"}`, "local_read: path must be relative to the workspace"},
		"empty old text":         {"edit", `{"path":"a.go","old_text":"","new_text":"x"}`, "local_edit: old_text must not be empty"},
		"replace with digest":    {"write", `{"path":"a.go","content":"x","mode":"replace","expected_sha256":"` + digest + `"}`, ""},
		"create":                 {"write", `{"path":"a.go","content":"x","mode":"create"}`, ""},
		"edit":                   {"edit", `{"path":"a.go","old_text":"a","new_text":"b"}`, ""},
	} {
		id, _ := domain.NewLocalToolIdentity(test.operation)
		tool := domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName("local_" + test.operation)}, Identity: id}
		pass := argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil })
		err := localArgumentError(pass, tool, hostJSON(t, test.arguments))
		if test.want == "" && err != nil || test.want != "" && (err == nil || err.Error() != test.want) {
			t.Fatalf("%s: %v, want %q", name, err, test.want)
		}
	}
}
