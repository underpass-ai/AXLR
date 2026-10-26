package dto

// ReceiptSkills names the catalog skills an execution was pinned to: the
// sha256 of their compact JSON and the pins themselves.
type ReceiptSkills struct {
	SHA256 string              `json:"sha256"`
	Pins   []ExecutionSkillPin `json:"pins"`
}
