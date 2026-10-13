package storage

type sessionTask struct {
	Plan      string            `json:"plan"`
	Task      string            `json:"task"`
	Scope     []string          `json:"scope,omitempty"`
	Protect   []string          `json:"protect,omitempty"`
	Program   string            `json:"check_program,omitempty"`
	Args      []string          `json:"check_args,omitempty"`
	TestFirst bool              `json:"test_first,omitempty"`
	Start     map[string]string `json:"start,omitempty"`
	Frozen    map[string]string `json:"frozen,omitempty"`
	Git       bool              `json:"git,omitempty"`
}
