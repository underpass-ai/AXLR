package storage

type sessionRepair struct {
	Improvement   bool     `json:"improvement,omitempty"`
	Repository    string   `json:"repository,omitempty"`
	Base          string   `json:"base,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	Slug          string   `json:"slug,omitempty"`
	Title         string   `json:"title,omitempty"`
	CauseRef      string   `json:"cause_ref,omitempty"`
	PullRequest   int      `json:"pull_request,omitempty"`
	URL           string   `json:"url,omitempty"`
	HeadSHA       string   `json:"head_sha,omitempty"`
	Rounds        int      `json:"rounds,omitempty"`
	Feedback      string   `json:"feedback,omitempty"`
	Cause         string   `json:"cause,omitempty"`
	Fix           string   `json:"fix,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Criteria      string   `json:"criteria,omitempty"`
	Scope         string   `json:"scope,omitempty"`
	WakeRefs      []string `json:"wake_refs,omitempty"`
	Awaiting      string   `json:"awaiting,omitempty"`
	Decided       string   `json:"decided,omitempty"`
	Granted       bool     `json:"granted,omitempty"`
	MergeSHA      string   `json:"merge_sha,omitempty"`
	CauseRecorded bool     `json:"cause_recorded,omitempty"`
}
