package axlr

import "github.com/underpass-ai/AXLR/domain"

type ResponseMapper struct{}

func (ResponseMapper) Map(result any) any {
	switch v := result.(type) {
	case domain.ReadResult:
		return ReadOutput{Content: v.Content, StartOffsetBytes: int64(v.StartOffset), ReturnedBytes: v.ReturnedBytes, NextOffsetBytes: int64(v.NextOffset), Truncated: v.Truncated, ContentSHA256: string(v.Digest)}
	case domain.WriteResult:
		return WriteOutput{WrittenBytes: v.WrittenBytes, ContentSHA256: string(v.Digest)}
	case domain.ExecResult:
		return ExecOutput{ExitCode: v.ExitCode, Stdout: v.Stdout, Stderr: v.Stderr, CapturedBytes: v.CapturedBytes, DiscardedBytes: v.DiscardedBytes, Truncated: v.Truncated}
	default:
		return nil
	}
}
