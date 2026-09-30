package application

// DiagnosticAction identifies a measured operation without user content.
type DiagnosticAction string

const (
	DiagnosticActionErrorDrain      DiagnosticAction = "error_response_drain"
	DiagnosticActionPluginDiscovery DiagnosticAction = "plugin_discovery"
	DiagnosticActionPluginPolicy    DiagnosticAction = "plugin_policy"
	DiagnosticActionContext         DiagnosticAction = "context_assembly"
	DiagnosticActionModel           DiagnosticAction = "model_generation"
	DiagnosticActionTools           DiagnosticAction = "tool_discovery"
	DiagnosticActionToolResolve     DiagnosticAction = "tool_resolution"
	DiagnosticActionToolExecution   DiagnosticAction = "tool_execution"
	DiagnosticActionSessionSave     DiagnosticAction = "session_save"
	DiagnosticActionSessionLoad     DiagnosticAction = "session_load"
	DiagnosticActionSessionList     DiagnosticAction = "session_list"
	DiagnosticActionCatalog         DiagnosticAction = "model_catalog"
	DiagnosticActionOperation       DiagnosticAction = "agent_operation"
	DiagnosticActionEventDelivery   DiagnosticAction = "event_delivery"
	DiagnosticActionRender          DiagnosticAction = "render"
	DiagnosticActionHTTP            DiagnosticAction = "http_exchange"
	DiagnosticActionUpdate          DiagnosticAction = "ui_update"
	DiagnosticActionPayload         DiagnosticAction = "payload_capture"
)

func (a DiagnosticAction) Valid() bool {
	switch a {
	case DiagnosticActionErrorDrain, DiagnosticActionPluginDiscovery, DiagnosticActionPluginPolicy, DiagnosticActionContext, DiagnosticActionModel, DiagnosticActionTools,
		DiagnosticActionToolResolve, DiagnosticActionToolExecution, DiagnosticActionSessionSave,
		DiagnosticActionSessionLoad, DiagnosticActionSessionList, DiagnosticActionCatalog,
		DiagnosticActionOperation, DiagnosticActionEventDelivery, DiagnosticActionRender,
		DiagnosticActionHTTP, DiagnosticActionUpdate, DiagnosticActionPayload:
		return true
	}
	return false
}
