package application

// DiagnosticStage is an allowlisted lifecycle checkpoint. It never contains
// prompts, model names, tool arguments, response text, or credentials.
type DiagnosticStage string

const (
	DiagnosticStartup           DiagnosticStage = "startup"
	DiagnosticShutdown          DiagnosticStage = "shutdown"
	DiagnosticModelCatalogStart DiagnosticStage = "model_catalog_start"
	DiagnosticModelCatalogDone  DiagnosticStage = "model_catalog_done"
	DiagnosticSessionLoad       DiagnosticStage = "session_load"
	DiagnosticSessionList       DiagnosticStage = "session_list"
	DiagnosticSessionSave       DiagnosticStage = "session_save"
	DiagnosticInputSubmitted    DiagnosticStage = "input_submitted"
	DiagnosticOperationStarted  DiagnosticStage = "operation_started"
	DiagnosticProviderStart     DiagnosticStage = "provider_start"
	DiagnosticProviderHeaders   DiagnosticStage = "provider_headers"
	DiagnosticProviderProgress  DiagnosticStage = "provider_progress"
	DiagnosticProviderDone      DiagnosticStage = "provider_done"
	DiagnosticEventEmitted      DiagnosticStage = "event_emitted"
	DiagnosticEventConsumed     DiagnosticStage = "event_consumed"
	DiagnosticRender            DiagnosticStage = "render"
	DiagnosticResize            DiagnosticStage = "resize"
	DiagnosticToolRequested     DiagnosticStage = "tool_requested"
	DiagnosticToolApproved      DiagnosticStage = "tool_approved"
	DiagnosticToolRejected      DiagnosticStage = "tool_rejected"
	DiagnosticToolCompleted     DiagnosticStage = "tool_completed"
	DiagnosticOperationDone     DiagnosticStage = "operation_done"
	DiagnosticOperationFailed   DiagnosticStage = "operation_failed"
)

func (stage DiagnosticStage) Valid() bool {
	switch stage {
	case DiagnosticStartup, DiagnosticShutdown,
		DiagnosticModelCatalogStart, DiagnosticModelCatalogDone,
		DiagnosticSessionLoad, DiagnosticSessionList, DiagnosticSessionSave, DiagnosticInputSubmitted,
		DiagnosticOperationStarted, DiagnosticProviderStart, DiagnosticProviderHeaders,
		DiagnosticProviderProgress, DiagnosticProviderDone,
		DiagnosticEventEmitted, DiagnosticEventConsumed,
		DiagnosticRender, DiagnosticResize,
		DiagnosticToolRequested, DiagnosticToolApproved, DiagnosticToolRejected,
		DiagnosticToolCompleted, DiagnosticOperationDone, DiagnosticOperationFailed:
		return true
	default:
		return false
	}
}
