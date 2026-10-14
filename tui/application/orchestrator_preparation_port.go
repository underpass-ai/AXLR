package application

import "context"

// OrchestratorPreparationPort prepares the configured local orchestrator so
// AXLR works under its own least-privilege identity. It is an explicit
// setup operation: it may act as the store's trusted host, which the work
// agent never is.
type OrchestratorPreparationPort interface {
	Prepare(context.Context) (OrchestratorPreparation, error)
	// Publish validates and publishes one definition the person wrote
	// (/ceremony publish), through the same temporary install grant as the
	// console's own definitions, and returns "name version".
	Publish(ctx context.Context, definitionYAML string) (string, error)
}
