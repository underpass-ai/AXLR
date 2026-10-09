package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Discovery and current-session bookkeeping are intrinsic. Wrappers retain target approval.
func automaticallyApproves(policy ToolApprovalPolicyPort, id domain.ToolIdentity) bool {
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationRemember {
		// It writes KMP memory: approved as kmp_write_memory would be.
		return policy != nil && policy.AutoApproves(domain.MemoryWriteIdentity)
	}
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationForgeTool {
		// It writes the tool's files: approved as local_write would be.
		return policy != nil && policy.AutoApproves(localIdentity("write"))
	}
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationRunTool {
		// It runs the tool's program: approved as local_exec would be.
		return policy != nil && policy.AutoApproves(localIdentity("exec"))
	}
	if id.Kind == domain.ToolKindHost {
		// A repair or improvement request starts a separate session the
		// console validates and drives; its own approvals (the check command,
		// the merge) stay with the person, so the request itself needs no
		// card. A judgement
		// asks Jev a question the person enabled in settings and changes
		// nothing. The console's log is read-only.
		return id.LocalOperation == domain.HostOperationLogs || id.LocalOperation == domain.HostOperationTools || id.LocalOperation == domain.HostOperationHistory || id.LocalOperation == domain.HostOperationSkill || id.LocalOperation == domain.HostOperationSession || id.LocalOperation == domain.HostOperationRequestRepair || id.LocalOperation == domain.HostOperationRepairStatus || id.LocalOperation == domain.HostOperationRequestImprovement || id.LocalOperation == domain.HostOperationJudge
	}
	return policy != nil && policy.AutoApproves(id)
}

// approvesInSession adds the session's ceremony to approvesInMode: a step
// result that proposes a new check command waits for the user; any other step
// result is the console's own bookkeeping.
func approvesInSession(policy ToolApprovalPolicyPort, s domain.Session, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationStepDone {
		return !stepDoneNeedsApproval(s, arguments) && !planNeedsApproval(s, arguments)
	}
	return approvesInMode(policy, s.Mode(), id, arguments)
}

// approvesInMode adds the work mode: a call the mode puts under the user's
// decision is never approved automatically, autonomy included.
func approvesInMode(policy ToolApprovalPolicyPort, mode domain.WorkMode, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if verdict, _ := mode.Judge(id, arguments); verdict != domain.VerdictAllow {
		return false
	}
	return automaticallyApproves(policy, id)
}
