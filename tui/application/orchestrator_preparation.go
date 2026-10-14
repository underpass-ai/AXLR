package application

// OrchestratorPreparation says what a preparation of the orchestrator did.
type OrchestratorPreparation struct {
	Status       string // ready, granted, remote, unsupported, missing, conflict
	WorkIdentity string
	// ApproverIdentity grants human guards when the person decides at the
	// console; the work identity cannot.
	ApproverIdentity string
	GrantID          string
	RestartRequired  bool
	Detail           string
	// Published lists the ceremony definitions this preparation published.
	Published []string
}
