package application

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// exactCalls approves the exec calls whose arguments it holds, in one
// session.
type exactCalls struct {
	session domain.SessionID
	args    string
}

func (exactCalls) AutoApproves(domain.ToolIdentity) bool { return false }

func (e exactCalls) ApprovesCall(session domain.SessionID, id domain.ToolIdentity, args root.JSONValue) bool {
	return session == e.session && id.Kind == domain.ToolKindLocal && id.LocalOperation == "exec" && string(args.Bytes()) == e.args
}

// A call the person allowed exactly for the session runs without a card,
// also where the mode asks for each command; another session, other
// arguments and a call the mode denies still wait or are refused.
func TestAnExactCallApprovalOutranksTheModesQuestionNotItsDenial(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeReview); err != nil {
		t.Fatal(err)
	}
	exec := domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "exec"}
	allowed := `{"program":"git","args":["log"]}`
	policy := exactCalls{session: s.ID(), args: allowed}
	value := func(raw string) root.JSONValue { v, _ := root.NewJSONValue([]byte(raw)); return v }
	if !approvesInSession(policy, s, exec, value(allowed)) {
		t.Fatal("the allowed command still waits in review mode")
	}
	if approvesInSession(policy, s, exec, value(`{"program":"git","args":["push"]}`)) {
		t.Fatal("another command was approved")
	}
	if approvesInSession(exactCalls{session: "ffffffffffffffffffffffffffffffff", args: allowed}, s, exec, value(allowed)) {
		t.Fatal("another session's allow applied")
	}
	stdin := `{"program":"git","args":["log"],"stdin":"x"}`
	if approvesInSession(exactCalls{session: s.ID(), args: stdin}, s, exec, value(stdin)) {
		t.Fatal("a call the mode denies was approved")
	}
}
