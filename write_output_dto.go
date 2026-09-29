package axlr

type WriteOutput struct {
	WrittenBytes  int    `json:"written_bytes"`
	ContentSHA256 string `json:"content_sha256"`
}
