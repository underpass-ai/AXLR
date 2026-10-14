package application

// RepairMarkerFile is the launcher's record of what the clone is for.
type RepairMarkerFile struct {
	Version    int    `json:"version"`
	Repository string `json:"repository"`
	Base       string `json:"base"`
	Slug       string `json:"slug"`
	Brief      string `json:"brief"`
	About      string `json:"about"`
	Issue      string `json:"issue,omitempty"`
	// Kind is ImproveKind for an improvement clone, empty for a repair.
	Kind    string `json:"kind,omitempty"`
	Created string `json:"created"`
	// Origin is the session that requested the repair from a running
	// console, empty for a launcher clone; Build the console build that
	// detected the defect.
	Origin string `json:"origin_session,omitempty"`
	Build  string `json:"build,omitempty"`
}
