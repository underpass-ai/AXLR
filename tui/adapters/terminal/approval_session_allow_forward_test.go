package terminal

import (
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type forwardedSettings struct {
	always  []string
	private bool
}

func (s forwardedSettings) AutoApproves(domain.ToolIdentity) bool { return true }
func (s forwardedSettings) AlwaysAsk() []string                   { return s.always }
func (s forwardedSettings) PrivateTmp() bool                      { return s.private }

// The console's policy passes on the person's destructive list and the
// sandbox's private /tmp: wrapped, every turn judged commands by every
// rule and a shared /tmp, so an emptied always_ask still asked, and rm in
// the sandbox's /tmp asked under autonomy (wrx80, 21 November 2026).
func TestTheConsolePolicyPassesOnTheDestructiveListAndThePrivateTmp(t *testing.T) {
	policy := allowingPolicy{settings: forwardedSettings{always: []string{domain.RuleGitPush}, private: true}, allowed: &commandAllows{}}
	always, ok := application.ToolApprovalPolicyPort(policy).(application.AlwaysAskPort)
	if !ok || len(always.AlwaysAsk()) != 1 || always.AlwaysAsk()[0] != domain.RuleGitPush {
		t.Fatalf("the destructive list was not passed on (%v)", ok)
	}
	private, ok := application.ToolApprovalPolicyPort(policy).(application.PrivateTmpPort)
	if !ok || !private.PrivateTmp() {
		t.Fatalf("the private /tmp was not passed on (%v)", ok)
	}
	if empty := (allowingPolicy{settings: forwardedSettings{always: []string{}}}).AlwaysAsk(); empty == nil || len(empty) != 0 {
		t.Fatalf("an emptied list became every rule: %v", empty)
	}
}
