package application

// CeremonyView is the part of a MADE instance the driver reconciles from.
type CeremonyView struct {
	State string
	// Enabled lists the triggers MADE would accept now.
	Enabled []string
	// Claimable lists the steps that can be claimed now.
	Claimable []string
	// Completed holds the outputs of the steps completed in this visit of
	// the current state, so a completion whose answer was lost is found.
	Completed map[string]map[string]any
	// Outputs holds the latest completed output of every step, so a resumed
	// console step finds what an earlier one recorded.
	Outputs map[string]map[string]any
	// Live maps a step with an unexpired claim of the console's own to its
	// fence: MADE refuses a second claim while the lease runs, so the
	// console completes with the original fence instead.
	Live map[string]string
	// LiveErr is why Live could not be read: MADE's resume inspection
	// failed, so whether a claim of the console's own is live is unknown,
	// and a console step must be neither claimed again nor passed over on
	// that guess.
	LiveErr error
}
