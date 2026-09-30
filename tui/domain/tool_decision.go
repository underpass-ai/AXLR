package domain

type ToolDecision string

const (
	DecisionApprove     ToolDecision = "approve"
	DecisionAutoApprove ToolDecision = "auto_approve"
	DecisionDeny        ToolDecision = "deny"
)
