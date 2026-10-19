package application

import (
	"context"
	"encoding/json"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Discovery and current-session bookkeeping are intrinsic. Wrappers retain target approval.
func automaticallyApproves(policy ToolApprovalPolicyPort, id domain.ToolIdentity) bool {
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationRemember {
		// It writes KMP memory: approved as kmp_write_memory would be.
		return policy != nil && policy.AutoApproves(domain.MemoryWriteIdentity)
	}
	if id.Kind == domain.ToolKindHost && (id.LocalOperation == domain.HostOperationForgeTool || id.LocalOperation == domain.HostOperationRemoveTool || id.LocalOperation == domain.HostOperationInstallTool) {
		// It writes or deletes the tool's files: approved as local_write
		// would be. An install writes the files the card showed.
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
		// nothing. The console's log and the catalogs' indexes are
		// read-only.
		return id.LocalOperation == domain.HostOperationLogs || id.LocalOperation == domain.HostOperationCatalog || id.LocalOperation == domain.HostOperationTools || id.LocalOperation == domain.HostOperationHistory || id.LocalOperation == domain.HostOperationSkill || id.LocalOperation == domain.HostOperationSession || id.LocalOperation == domain.HostOperationRequestRepair || id.LocalOperation == domain.HostOperationRepairStatus || id.LocalOperation == domain.HostOperationRequestImprovement || id.LocalOperation == domain.HostOperationJudge || id.LocalOperation == domain.HostOperationRecall
	}
	if readsWorkspace(id) {
		// On 10 October 2026, with autonomy off, every local_search and
		// local_read of claude-haiku-5.5 raised a card, so a "where is X"
		// question cost several keypresses before the model could look.
		// Reading changes nothing; approvesInMode still asks the mode first.
		return true
	}
	return policy != nil && policy.AutoApproves(id)
}

// readsWorkspace reports the local tools that only read the workspace.
func readsWorkspace(id domain.ToolIdentity) bool {
	return id.Kind == domain.ToolKindLocal && (id.LocalOperation == "read" || id.LocalOperation == "search" || id.LocalOperation == "list")
}

// approvesInSession adds the session's ceremony to approvesInMode: a step
// result that proposes a new check command waits for the user; any other step
// result is the console's own bookkeeping.
func approvesInSession(policy ToolApprovalPolicyPort, s domain.Session, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationStepDone {
		return !stepDoneNeedsApproval(s, arguments) && !planNeedsApproval(s, arguments)
	}
	if DestructiveReason(policy, s, id, arguments) != "" {
		return false // the destructive list keeps its card (decision K5a)
	}
	if calls, ok := policy.(CallApprovalPort); ok && calls.ApprovesCall(s.ID(), id, arguments) {
		// The person allowed this exact call for the session, where the
		// mode asks for each one too; a mode's denial still holds.
		if verdict, _ := s.Mode().Judge(id, arguments); verdict != domain.VerdictDeny {
			return true
		}
	}
	return approvesInMode(policy, s.Mode(), id, arguments)
}

// approvesCall is approvesInSession plus what only the ceremony driver can
// tell, reading the workspace: a decompose hand-back the console will return
// for its defects without running a command needs no card for its commands.
// withheld is true in that case, and the caller marks the context with
// withholdCommands so the commands still do not run if the plan passes when
// decompose reads it again.
func approvesCall(ctx context.Context, policy ToolApprovalPolicyPort, ceremonies *CeremonyDriver, s domain.Session, id domain.ToolIdentity, arguments root.JSONValue) (approved, withheld bool) {
	if approvesInSession(policy, s, id, arguments) {
		return true, false
	}
	if id.Kind == domain.ToolKindHost && id.LocalOperation == domain.HostOperationStepDone && ceremonies.refusesPlanBeforeItsCommands(ctx, s, arguments) {
		return true, true
	}
	return false, false
}

// approvesInMode adds the work mode: a call the mode puts under the user's
// decision is never approved automatically, autonomy included.
func approvesInMode(policy ToolApprovalPolicyPort, mode domain.WorkMode, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if verdict, _ := mode.Judge(id, arguments); verdict != domain.VerdictAllow {
		return false
	}
	return automaticallyApproves(policy, id)
}

// AlwaysAskPort names the destructive rules in force (settings
// approvals.always_ask); a policy without it keeps every rule.
type AlwaysAskPort interface {
	AlwaysAsk() []string
}

// DestructiveReason names the destructive rule a local_exec call falls
// under, "" for any other call: such a call keeps its approval card under
// autonomy and under a saved rule (decision K5a).
func DestructiveReason(policy ToolApprovalPolicyPort, s domain.Session, id domain.ToolIdentity, arguments root.JSONValue) string {
	if id.Kind != domain.ToolKindLocal || id.LocalOperation != "exec" {
		return ""
	}
	// The rules read the command as it would run: a tolerant session splits
	// "git push --force origin main" sent as the program before running it
	// (normalizeExec), which the rules missed until 14 October 2026. The
	// split applies in every session here: a card for a command that would
	// not run costs a keypress.
	arguments, _ = normalizeExec(arguments)
	var call struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	if json.Unmarshal(arguments.Bytes(), &call) != nil || call.Program == "" {
		return ""
	}
	var enabled []string
	if asks, ok := policy.(AlwaysAskPort); ok {
		enabled = asks.AlwaysAsk()
	}
	return domain.DestructiveReason(s.Export().Workspace, call.Program, call.Args, call.Cwd, enabled)
}
