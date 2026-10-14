package storage

// RepairSettings is the repair section of settings.json. Repository is
// owner/name; an empty Directory means <data>/axlr/repairs; WatchMinutes
// bounds one check round (default 45); About is the KMP project about
// (default project:<name>). Autonomous lets a repair session the agent
// started run local tools in its clone without a card (default true; the
// check command and the merge keep their approvals); MaxAttempts bounds the
// repair sessions one failure may start (default 2, at most 5).
type RepairSettings struct {
	Repository   string `json:"repository"`
	Directory    string `json:"directory,omitempty"`
	AutoMerge    bool   `json:"auto_merge"`
	WatchMinutes int    `json:"watch_minutes,omitempty"`
	About        string `json:"about,omitempty"`
	Autonomous   *bool  `json:"autonomous,omitempty"`
	MaxAttempts  int    `json:"max_attempts,omitempty"`
	// Install replaces the running axlr-tui with the repaired one once its
	// pull request merges; off by default, when the panel's i does it.
	Install bool `json:"install,omitempty"`
}

// AutonomousLocal reports whether an agent-started repair session runs local
// tools in its clone without the person's card.
func (r RepairSettings) AutonomousLocal() bool {
	return r.Autonomous == nil || *r.Autonomous
}
