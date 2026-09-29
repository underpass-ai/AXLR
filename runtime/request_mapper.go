package runtime

import (
	"errors"
	"time"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

type RequestMapper struct {
	MaxReadBytes   int
	MaxFileBytes   int
	MaxOutputBytes int
	MaxTimeout     time.Duration
}

func (m RequestMapper) Map(req dto.Request) (any, error) {
	if req.ProtocolVersion != ProtocolVersion {
		return nil, errors.New("unsupported protocol version")
	}
	if _, err := domain.NewRequestID(req.RequestID); err != nil {
		return nil, err
	}
	if len(req.Arguments) == 0 || len(req.Arguments) > MaxRequestBytes {
		return nil, errors.New("invalid arguments")
	}
	switch req.Tool {
	case "read":
		var a dto.ReadArgs
		if err := strictJSON(req.Arguments, &a); err != nil {
			return nil, err
		}
		path, err := domain.NewRelativePath(a.Path)
		if err != nil {
			return nil, err
		}
		offset, err := domain.NewByteOffset(a.OffsetBytes)
		if err != nil {
			return nil, err
		}
		max := a.MaxBytes
		if max == 0 {
			max = defaultReadBytes
			if max > m.MaxReadBytes {
				max = m.MaxReadBytes
			}
		}
		limit, err := domain.NewByteLimit(max, m.MaxReadBytes)
		if err != nil {
			return nil, err
		}
		return domain.ReadCommand{Path: path, Offset: offset, Limit: limit}, nil
	case "write":
		var a dto.WriteArgs
		if err := strictJSON(req.Arguments, &a); err != nil {
			return nil, err
		}
		path, err := domain.NewRelativePath(a.Path)
		if err != nil {
			return nil, err
		}
		content, err := domain.NewText(a.Content)
		if err != nil {
			return nil, err
		}
		if len(content) > m.MaxFileBytes {
			return nil, errors.New("content exceeds editable limit")
		}
		mode, err := domain.NewWriteMode(a.Mode)
		if err != nil {
			return nil, err
		}
		var expected domain.Digest
		if a.ExpectedSHA256 != "" {
			expected, err = domain.NewDigest(a.ExpectedSHA256)
			if err != nil {
				return nil, err
			}
		}
		return domain.WriteCommand{Path: path, Content: content, Mode: mode, ExpectedDigest: expected}, nil
	case "edit":
		var a dto.EditArgs
		if err := strictJSON(req.Arguments, &a); err != nil {
			return nil, err
		}
		path, err := domain.NewRelativePath(a.Path)
		if err != nil {
			return nil, err
		}
		old, err := domain.NewText(a.OldText)
		if err != nil {
			return nil, err
		}
		if old == "" {
			return nil, errors.New("old_text must not be empty")
		}
		next, err := domain.NewText(a.NewText)
		if err != nil {
			return nil, err
		}
		var expected domain.Digest
		if a.ExpectedSHA256 != "" {
			expected, err = domain.NewDigest(a.ExpectedSHA256)
			if err != nil {
				return nil, err
			}
		}
		return domain.EditCommand{Path: path, OldText: old, NewText: next, ExpectedDigest: expected}, nil
	case "exec":
		var a dto.ExecArgs
		if err := strictJSON(req.Arguments, &a); err != nil {
			return nil, err
		}
		program, err := domain.NewProgram(a.Program)
		if err != nil {
			return nil, err
		}
		cwd, err := domain.NewWorkingDirectory(a.Cwd)
		if err != nil {
			return nil, err
		}
		stdin, err := domain.NewText(a.Stdin)
		if err != nil {
			return nil, err
		}
		argv, err := domain.NewArgv(a.Args)
		if err != nil {
			return nil, err
		}
		if a.TimeoutMS < 0 || a.TimeoutMS > m.MaxTimeout.Milliseconds() {
			return nil, errors.New("timeout exceeds profile limit")
		}
		duration := time.Duration(a.TimeoutMS) * time.Millisecond
		if a.TimeoutMS == 0 {
			duration = 30 * time.Second
			if duration > m.MaxTimeout {
				duration = m.MaxTimeout
			}
		}
		timeout, err := domain.NewTimeout(duration, m.MaxTimeout)
		if err != nil {
			return nil, err
		}
		outputMax := a.MaxOutputBytes
		if outputMax == 0 {
			outputMax = defaultOutputBytes
			if outputMax > m.MaxOutputBytes {
				outputMax = m.MaxOutputBytes
			}
		}
		outputLimit, err := domain.NewByteLimit(outputMax, m.MaxOutputBytes)
		if err != nil {
			return nil, err
		}
		return domain.ExecCommand{Program: program, Args: argv, Cwd: cwd, Stdin: stdin, Timeout: timeout, OutputLimit: outputLimit}, nil
	default:
		return nil, errors.New("unknown tool")
	}
}
