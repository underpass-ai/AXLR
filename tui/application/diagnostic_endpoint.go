package application

// DiagnosticEndpoint distinguishes provider operations without storing raw URLs.
type DiagnosticEndpoint string

const (
	DiagnosticEndpointChat   DiagnosticEndpoint = "chat_completions"
	DiagnosticEndpointModels DiagnosticEndpoint = "model_catalog"
	DiagnosticEndpointOther  DiagnosticEndpoint = "other_provider"
)

func (e DiagnosticEndpoint) Valid() bool {
	return e == DiagnosticEndpointChat || e == DiagnosticEndpointModels || e == DiagnosticEndpointOther
}
