package storage

// JevSettings is the jev section. Tool offers axlr_judge to the model;
// FinalCheck asks Jev whether a final answer completes the request and
// returns a doubted answer to the model once; FinalThreshold is the
// probability below which it does (default 0.5). Model is the pinned Jev
// version (default jev-1.13.0, as KMP); TimeoutMS bounds one request
// (default 20000). The key is read from TYPESAFE_API_KEY.
type JevSettings struct {
	Tool           bool    `json:"tool"`
	FinalCheck     bool    `json:"final_check"`
	FinalThreshold float64 `json:"final_threshold,omitempty"`
	Model          string  `json:"model,omitempty"`
	TimeoutMS      int     `json:"timeout_ms,omitempty"`
}

// Enabled reports whether either switch sends anything to TypeSafe.
func (j *JevSettings) Enabled() bool { return j != nil && (j.Tool || j.FinalCheck) }
