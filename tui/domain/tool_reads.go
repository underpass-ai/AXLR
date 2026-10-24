package domain

// ReadsOnly reports the tools that only read: the local reads of the
// workspace and the host tools that list, describe or recall without an
// effect outside the console (axlr_session names the session, console
// bookkeeping). It is the one list the execution policy's observe level
// stands on. An MCP tool is never a read here: MCP does not tell the
// console which of a server's tools read, so the policy's allow list
// vouches for the ones it names.
func ReadsOnly(id ToolIdentity) bool {
	switch id.Kind {
	case ToolKindLocal:
		return id.LocalOperation == "read" || id.LocalOperation == "search" || id.LocalOperation == "list"
	case ToolKindHost:
		switch id.LocalOperation {
		case HostOperationTools, HostOperationHistory, HostOperationSkill, HostOperationSession, HostOperationLogs, HostOperationCatalog, HostOperationRepairStatus, HostOperationRecall, HostOperationJudge:
			return true
		}
	}
	return false
}
