package axlr

type WriteArgs struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	Mode           string `json:"mode"`
	ExpectedSHA256 string `json:"expected_sha256"`
}
