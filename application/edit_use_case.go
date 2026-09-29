package application

import (
	"bytes"
	"github.com/underpass-ai/AXLR/domain"
)

type EditUseCase struct {
	Files        FilePort
	MaxFileBytes int
}

func (u EditUseCase) Execute(c domain.EditCommand) (domain.WriteResult, error) {
	if c.OldText == "" {
		return domain.WriteResult{}, domain.Reject("invalid_arguments", "old_text must not be empty")
	}
	snapshot, err := u.Files.Load(c.Path, u.MaxFileBytes)
	if err != nil {
		return domain.WriteResult{}, err
	}
	if c.ExpectedDigest != "" && domain.DigestOf(snapshot.Content) != c.ExpectedDigest {
		return domain.WriteResult{}, domain.Reject("conflict", "content digest does not match")
	}
	if bytes.Count(snapshot.Content, []byte(c.OldText)) != 1 {
		return domain.WriteResult{}, domain.Reject("conflict", "old_text must occur exactly once")
	}
	after := bytes.Replace(snapshot.Content, []byte(c.OldText), []byte(c.NewText), 1)
	if len(after) > u.MaxFileBytes {
		return domain.WriteResult{}, domain.Reject("file_too_large", "edited content exceeds editable limit")
	}
	if err := u.Files.Replace(c.Path, after, snapshot.Permissions); err != nil {
		return domain.WriteResult{}, err
	}
	return domain.WriteResult{WrittenBytes: len(after), Digest: domain.DigestOf(after)}, nil
}
