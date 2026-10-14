package main

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
)

// planWorkbenches is application.PlanWorkbenchPort: a workbench rooted at
// the plan's workspace with autonomous local tools, since workers run
// without the person once the plan is approved.
type planWorkbenches struct{ repairWorkbenches }

var _ application.PlanWorkbenchPort = planWorkbenches{}

func (w planWorkbenches) Open(ctx context.Context, workspace string) (application.PlanWorkbench, error) {
	w.autonomous = true
	bench, err := w.repairWorkbenches.Open(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return bench.(*application.UseCaseWorkbench), nil
}
