package dto

// ReceiptRouting names the routing an execution ran under: the sha256 of
// its compact JSON, its objective, its candidates and whether remote
// providers were allowed.
type ReceiptRouting struct {
	SHA256               string   `json:"sha256"`
	Objective            string   `json:"objective"`
	Candidates           []string `json:"candidates"`
	AllowRemoteProviders bool     `json:"allow_remote_providers"`
}
