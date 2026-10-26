package dto

// ExecutionRouting is the runtime contract's routing per execution
// (docs/runtime-contract.md): what `axlr-tui --exec --routing` reads and
// what CreateSession takes and GetSession shows. A zero or absent
// constraint is none; allow_remote_providers absent allows them.
type ExecutionRouting struct {
	Candidates           []string `json:"candidates"`
	Objective            string   `json:"objective"`
	AllowRemoteProviders *bool    `json:"allow_remote_providers,omitempty"`
	MaxContextTokens     int      `json:"max_context_tokens,omitempty"`
	MaxRequestUSD        float64  `json:"max_request_usd,omitempty"`
}
