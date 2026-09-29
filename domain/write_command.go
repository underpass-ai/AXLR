package domain

type WriteCommand struct {
	Path           RelativePath
	Content        Text
	Mode           WriteMode
	ExpectedDigest Digest
}
