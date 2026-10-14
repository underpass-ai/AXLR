package storage

type sessionPlan struct {
	ID           string   `json:"id"`
	Awaiting     string   `json:"awaiting,omitempty"`
	Decided      string   `json:"decided,omitempty"`
	Granted      bool     `json:"granted,omitempty"`
	Returns      int      `json:"returns,omitempty"`
	ReturnReason string   `json:"return_reason,omitempty"`
	Defects      []string `json:"defects,omitempty"`
}
