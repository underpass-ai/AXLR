package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// policyVerdict is the session's execution policy on one resolved call;
// a session without a policy allows everything to the rest of the chain.
func policyVerdict(s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) (domain.PolicyVerdict, string) {
	policy, has := s.Policy()
	if !has {
		return domain.PolicyAllow, ""
	}
	return policy.Judge(tool, arguments)
}

// policyRefusal is the PolicyDenial of a call the session's policy forbids.
func policyRefusal(s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) (error, bool) {
	verdict, reason := policyVerdict(s, tool, arguments)
	if verdict != domain.PolicyForbid {
		return nil, false
	}
	return PolicyDenial{Reason: reason}, true
}

// policyAsks reports a call the session's policy puts under a decision
// whatever the run's autonomy: an allowed write under act-with-approval.
func policyAsks(s domain.Session, id domain.ToolIdentity, arguments root.JSONValue) bool {
	policy, has := s.Policy()
	if !has {
		return false
	}
	tool, known := toolByIdentity(s.ToolSnapshot(), id)
	if !known {
		return false
	}
	verdict, _ := policy.Judge(tool, arguments)
	return verdict != domain.PolicyAllow
}

// policyOffered is the snapshot without the tools the session's policy
// forbids by name: the model is not offered what it may not call.
func policyOffered(s domain.Session, snapshot []domain.AvailableTool) []domain.AvailableTool {
	policy, has := s.Policy()
	if !has {
		return snapshot
	}
	return policy.OfferedTools(snapshot)
}

func toolByIdentity(snapshot []domain.AvailableTool, id domain.ToolIdentity) (domain.AvailableTool, bool) {
	for _, tool := range snapshot {
		if tool.Identity == id {
			return tool, true
		}
	}
	return domain.AvailableTool{}, false
}
