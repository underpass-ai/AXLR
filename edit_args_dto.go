package axlr

type EditArgs struct {
	Path           string `json:"path"`
	OldText        string `json:"old_text"`
	NewText        string `json:"new_text"`
	ExpectedSHA256 string `json:"expected_sha256"`
}
