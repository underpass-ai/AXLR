package storage

// sessionGeneric is a definition driven through /ceremony: each step's
// prompt and the terminal states.
type sessionGeneric struct {
	Prompts  map[string]string `json:"prompts"`
	Terminal []string          `json:"terminal,omitempty"`
}
