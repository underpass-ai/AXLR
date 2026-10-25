package dto

// ReceiptBudget names the budget an execution ran under: the sha256 of its
// compact JSON and its limits.
type ReceiptBudget struct {
	SHA256 string          `json:"sha256"`
	Limits ExecutionBudget `json:"limits"`
}
