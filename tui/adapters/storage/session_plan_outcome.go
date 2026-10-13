package storage

type sessionPlanOutcome struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"`
	Tasks     int                  `json:"tasks"`
	Done      int                  `json:"done"`
	Blocked   int                  `json:"blocked"`
	Skipped   int                  `json:"skipped"`
	Syncs     []sessionSyncVerdict `json:"syncs,omitempty"`
	Changed   []string             `json:"changed,omitempty"`
	CostUSD   float64              `json:"cost_usd,omitempty"`
	CostKnown bool                 `json:"cost_known,omitempty"`
}
