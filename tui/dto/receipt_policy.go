package dto

// ReceiptPolicy names the policy an execution ran under: its level and the
// sha256 of its compact JSON.
type ReceiptPolicy struct {
	Level  string `json:"level"`
	SHA256 string `json:"sha256"`
}
