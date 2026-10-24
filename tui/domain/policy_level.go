package domain

import "errors"

// PolicyLevel is the autonomy of one execution (docs/runtime-contract.md,
// "Policy per execution"): what its allowed tools may do without a
// decision.
type PolicyLevel string

const (
	// PolicyObserve runs reads only: the local reads, the read-only host
	// tools and the MCP tools the allow list names.
	PolicyObserve PolicyLevel = "observe"
	// PolicyPropose is observe plus writes of a plan or a diff in the
	// workspace (a document, or a .diff or .patch file).
	PolicyPropose PolicyLevel = "propose"
	// PolicyActWithApproval runs the allowed writes, each with a decision.
	PolicyActWithApproval PolicyLevel = "act-with-approval"
	// PolicyActWithinPolicy runs the allowed writes without a decision of
	// its own; the run's approval level, the settings' rules, the work mode
	// and the destructive list still decide.
	PolicyActWithinPolicy PolicyLevel = "act-within-policy"
)

func ParsePolicyLevel(raw string) (PolicyLevel, error) {
	level := PolicyLevel(raw)
	return level, level.Validate()
}

func (l PolicyLevel) Validate() error {
	switch l {
	case PolicyObserve, PolicyPropose, PolicyActWithApproval, PolicyActWithinPolicy:
		return nil
	}
	return errors.New("policy level must be observe, propose, act-with-approval or act-within-policy")
}
