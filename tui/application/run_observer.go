package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// runObserver mirrors the driver's progress into the record while a console
// step runs for minutes, so the panel shows propose, watch and decide.
type runObserver struct {
	repair *SelfRepair
	run    *repairRun
}

func (o runObserver) Observe(progress CeremonyProgress) {
	o.repair.mu.Lock()
	o.run.progress = progress
	o.repair.mu.Unlock()
	o.repair.update(context.Background(), o.run, func(record *domain.RepairRecord) {
		if progress.Instance != "" {
			record.Instance = progress.Instance
		}
		if progress.Step != "" {
			record.Step, record.StepAttempt, record.StepLimit = progress.Step, progress.StepAttempt, progress.StepLimit
		}
		if check := checkSummary(progress.Report); check != "" {
			record.Check = check
		}
		if progress.State != "" {
			record.State = progress.State
		}
		if repair := progress.Repair; repair != nil && repair.PullRequest > 0 {
			record.PullRequest, record.URL = repair.PullRequest, repair.URL
		}
		if memory, _ := progress.Report["memory"].(string); memory != "" {
			record.Memory = memory
		}
	})
}
