package domain

type EditCommand struct {
	Path           RelativePath
	OldText        Text
	NewText        Text
	ExpectedDigest Digest
}
