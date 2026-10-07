package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// PlanRegistryPort keeps the plans: the proposals, their tasks and syncs.
type PlanRegistryPort interface {
	Load(context.Context) ([]domain.PlanRecord, error)
	Save(context.Context, domain.PlanRecord) error
}

// PlanSettings configures /plan: the model that decomposes the brief and
// whether a verified plan starts without the person.
type PlanSettings struct {
	// Planner is the model the decompose step asks; empty means the
	// session's model.
	Planner string
	// AutoApprove records automatic instead of waiting for the person.
	AutoApprove bool
}
