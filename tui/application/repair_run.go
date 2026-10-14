package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// repairRun is one repair session the console drives in the background.
type repairRun struct {
	cancel    context.CancelFunc
	decisions chan repairDecision
	// record, waiting and progress are guarded by SelfRepair.mu.
	record   domain.RepairRecord
	waiting  bool
	progress CeremonyProgress
}
