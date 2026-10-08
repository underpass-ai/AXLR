package application

// DiagnosticStage is an allowlisted lifecycle checkpoint. It never contains
// prompts, model names, tool arguments, response text, or credentials.
type DiagnosticStage string

const (
	DiagnosticContextProjected  DiagnosticStage = "context_projected"
	DiagnosticActionStart       DiagnosticStage = "action_start"
	DiagnosticActionEnd         DiagnosticStage = "action_end"
	DiagnosticRequestSent       DiagnosticStage = "request_sent"
	DiagnosticWireBytes         DiagnosticStage = "provider_wire_bytes"
	DiagnosticFrame             DiagnosticStage = "provider_frame"
	DiagnosticReasoning         DiagnosticStage = "provider_reasoning"
	DiagnosticToolDelta         DiagnosticStage = "provider_tool_delta"
	DiagnosticContent           DiagnosticStage = "provider_content"
	DiagnosticHeartbeat         DiagnosticStage = "provider_heartbeat"
	DiagnosticWireDone          DiagnosticStage = "provider_wire_done"
	DiagnosticPayloadSaved      DiagnosticStage = "payload_saved"
	DiagnosticPayloadFailed     DiagnosticStage = "payload_failed"
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
	// DiagnosticJudgement records one Jev final check: its time and whether
	// it failed, never the verdict's text.
	DiagnosticJudgement DiagnosticStage = "judgement"
	// DiagnosticCeremonyStalled records a ceremony the console cancelled
	// because its model stopped handing the step back, or the person stopped.
	DiagnosticCeremonyStalled DiagnosticStage = "ceremony_stalled"
	// DiagnosticPromptQueued records a message the person sent while an
	// operation was running; DiagnosticSteerApplied records it joining the
	// running turn after a tool step, and DiagnosticSteerCancelled the
	// operation the console stopped for it instead.
	DiagnosticPromptQueued   DiagnosticStage = "prompt_queued"
	DiagnosticSteerApplied   DiagnosticStage = "steer_applied"
	DiagnosticSteerCancelled DiagnosticStage = "steer_cancelled"
)

func (stage DiagnosticStage) Valid() bool {
	switch stage {
	case DiagnosticContextProjected:
		return true
	case DiagnosticActionStart, DiagnosticActionEnd, DiagnosticRequestSent, DiagnosticWireBytes, DiagnosticFrame, DiagnosticReasoning,
		DiagnosticToolDelta, DiagnosticContent, DiagnosticHeartbeat, DiagnosticWireDone,
		DiagnosticPayloadSaved, DiagnosticPayloadFailed,
		DiagnosticStartup, DiagnosticShutdown,
		DiagnosticModelCatalogStart, DiagnosticModelCatalogDone,
		DiagnosticSessionLoad, DiagnosticSessionList, DiagnosticSessionSave, DiagnosticInputSubmitted,
		DiagnosticOperationStarted, DiagnosticProviderStart, DiagnosticProviderHeaders,
		DiagnosticProviderProgress, DiagnosticProviderDone,
		DiagnosticEventEmitted, DiagnosticEventConsumed,
		DiagnosticRender, DiagnosticResize,
		DiagnosticToolRequested, DiagnosticToolApproved, DiagnosticToolRejected,
		DiagnosticToolCompleted, DiagnosticOperationDone, DiagnosticOperationFailed,
		DiagnosticJudgement, DiagnosticCeremonyStalled,
		DiagnosticPromptQueued, DiagnosticSteerApplied, DiagnosticSteerCancelled:
		return true
	default:
		return false
	}
}
