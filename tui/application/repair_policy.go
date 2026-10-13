package application

import "time"

// RepairPolicy is what settings.json decides about the repair ceremony.
type RepairPolicy struct {
	// AutoMerge lets the console merge a green pull request without the
	// person; otherwise the merge waits on the approval card.
	AutoMerge bool
	// WatchDeadline bounds one check round; Poll spaces the status reads.
	WatchDeadline, Poll time.Duration
}
