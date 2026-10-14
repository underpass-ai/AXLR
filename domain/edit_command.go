package domain

type EditCommand struct {
	Path           RelativePath
	OldText        Text
	NewText        Text
	ExpectedDigest Digest
}

// CheckOldText refuses an edit without text to replace; it reads no file,
// so the console runs it before an approval card too.
func CheckOldText(old Text) error {
	if old == "" {
		return Reject("invalid_arguments", "old_text must not be empty")
	}
	return nil
}
