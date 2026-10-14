package application

import "context"

// CeremonyDefinitionPort reads published definitions.
type CeremonyDefinitionPort interface {
	Describe(ctx context.Context, name, version string) (CeremonyDefinition, error)
	List(ctx context.Context) ([]CeremonyDefinitionSummary, error)
}
