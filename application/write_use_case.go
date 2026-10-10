package application

import "github.com/underpass-ai/AXLR/domain"

type WriteUseCase struct {
	Files        FilePort
	MaxFileBytes int
}

func (u WriteUseCase) Execute(c domain.WriteCommand) (domain.WriteResult, error) {
	b := []byte(c.Content)
	if len(b) > u.MaxFileBytes {
		return domain.WriteResult{}, domain.Reject("file_too_large", "content exceeds editable limit")
	}
	switch c.Mode {
	case domain.CreateMode:
		if c.ExpectedDigest != "" {
			return domain.WriteResult{}, domain.Reject("invalid_arguments", "create does not accept expected_sha256")
		}
		if err := u.Files.Create(c.Path, b); err != nil {
			return domain.WriteResult{}, err
		}
	case domain.ReplaceMode:
		if c.ExpectedDigest == "" {
			// On 10 October 2026 a plan worker sent replace without the
			// digest and, told only that a precondition was missing, fell
			// back to local_edit: the message names the field and its source.
			return domain.WriteResult{}, domain.Reject("invalid_arguments", "replace requires expected_sha256: pass the content_sha256 of a local_read of "+string(c.Path))
		}
		snapshot, err := u.Files.Load(c.Path, u.MaxFileBytes)
		if err != nil {
			return domain.WriteResult{}, err
		}
		if domain.DigestOf(snapshot.Content) != c.ExpectedDigest {
			return domain.WriteResult{}, domain.Reject("conflict", "content digest does not match")
		}
		if err := u.Files.Replace(c.Path, b, snapshot.Permissions); err != nil {
			return domain.WriteResult{}, err
		}
	default:
		return domain.WriteResult{}, domain.Reject("invalid_arguments", "mode must be create or replace")
	}
	return domain.WriteResult{WrittenBytes: len(b), Digest: domain.DigestOf(b)}, nil
}
