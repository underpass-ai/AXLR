package domain

type WriteCommand struct {
	Path           RelativePath
	Content        Text
	Mode           WriteMode
	ExpectedDigest Digest
}

// CheckDigestForMode is the part of a write's validation that reads no
// file: create takes no expected digest, replace requires one. The write
// runs it, and the console runs it before an approval card: on 14 October
// 2026 a replace without expected_sha256 raised a card, the person
// approved it, and only then was it refused.
func CheckDigestForMode(path RelativePath, mode WriteMode, expected Digest) error {
	switch {
	case mode == CreateMode && expected != "":
		return Reject("invalid_arguments", "create does not accept expected_sha256")
	case mode == ReplaceMode && expected == "":
		// On 10 October 2026 a plan worker sent replace without the
		// digest and, told only that a precondition was missing, fell
		// back to local_edit: the message names the field and its source.
		return Reject("invalid_arguments", "replace requires expected_sha256: pass the content_sha256 of a local_read of "+string(path))
	}
	return nil
}
