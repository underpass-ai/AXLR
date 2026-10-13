package storage

type sessionIncident struct {
	Slug           string   `json:"slug,omitempty"`
	Service        string   `json:"service,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	DraftPath      string   `json:"draft_path,omitempty"`
	DraftDigest    string   `json:"draft_digest,omitempty"`
	Findings       []string `json:"findings,omitempty"`
	ReturnReason   string   `json:"return_reason,omitempty"`
	Returns        int      `json:"returns,omitempty"`
	ReviewFailures int      `json:"review_failures,omitempty"`
	Awaiting       string   `json:"awaiting,omitempty"`
	Decided        string   `json:"decided,omitempty"`
	Granted        bool     `json:"granted,omitempty"`
	Published      string   `json:"published,omitempty"`
}
