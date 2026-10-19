package storage

// JevSettings is the jev section. Tool offers axlr_judge to the model;
// FinalCheck asks Jev whether a final answer completes the request and
// returns a doubted answer to the model once; FinalThreshold is the
// probability below which it does (default 0.5). Model is the pinned Jev
// version (default jev-1.13.0, as KMP); TimeoutMS bounds one request
// (default 20000). The key is read from TYPESAFE_API_KEY.
//
// JudgePolicy selects how the final check and the settlement ask Jev:
// empty or "legacy" as until v0.11.0, "axlr-v2" the JEV lab's AXLR v2
// (docs/jev-v2-integration.md). It turns nothing on: the switches above
// decide whether Jev is asked at all. V2Supersede lets axlr-v2 ask whether
// a settled memory replaces a recalled one; it is off by default, for
// evaluation only, and refused under any other policy.
type JevSettings struct {
	Tool           bool    `json:"tool"`
	FinalCheck     bool    `json:"final_check"`
	FinalThreshold float64 `json:"final_threshold,omitempty"`
	Model          string  `json:"model,omitempty"`
	TimeoutMS      int     `json:"timeout_ms,omitempty"`
	JudgePolicy    string  `json:"judge_policy,omitempty"`
	V2Supersede    bool    `json:"v2_supersede,omitempty"`
}

// Enabled reports whether either switch sends anything to TypeSafe.
func (j *JevSettings) Enabled() bool { return j != nil && (j.Tool || j.FinalCheck) }
