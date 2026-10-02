package ceremonyhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Files is application.WorkspaceFilesPort over AXLR's local runtime, so the
// console's reads and writes keep the same workspace boundary as the model's.
type Files struct{ Tools application.ToolExecutionPort }

var _ application.WorkspaceFilesPort = Files{}

// maxDigestRead is below the runtime's 1 MiB file limit.
const maxDigestRead = 1<<20 - 1

type envelope struct {
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (f Files) local(ctx context.Context, operation string, arguments map[string]any) (envelope, error) {
	id, err := domain.NewLocalToolIdentity(operation)
	if err != nil {
		return envelope{}, err
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return envelope{}, err
	}
	value, err := root.NewJSONObject(encoded)
	if err != nil {
		return envelope{}, err
	}
	outcome, err := f.Tools.Execute(ctx, id, value)
	if err != nil {
		return envelope{}, err
	}
	var result envelope
	if err := json.Unmarshal([]byte(outcome.Content), &result); err != nil {
		return envelope{}, fmt.Errorf("unreadable %s result", operation)
	}
	return result, nil
}

func (f Files) Read(ctx context.Context, path string, maxBytes int) ([]byte, bool, error) {
	var content []byte
	offset := 0
	for len(content) < maxBytes {
		result, err := f.local(ctx, "read", map[string]any{"path": path, "offset_bytes": offset, "max_bytes": maxBytes - len(content)})
		if err != nil {
			return nil, false, err
		}
		if result.Error != nil {
			if result.Error.Code == "not_found" {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("read %s: %s", path, result.Error.Message)
		}
		var page struct {
			Content    string `json:"content"`
			Returned   int    `json:"returned_bytes"`
			NextOffset int    `json:"next_offset_bytes"`
			Truncated  bool   `json:"truncated"`
		}
		if err := json.Unmarshal(result.Output, &page); err != nil {
			return nil, false, fmt.Errorf("read %s: unreadable page", path)
		}
		content = append(content, page.Content...)
		if !page.Truncated || page.Returned == 0 {
			break
		}
		offset = page.NextOffset
	}
	return content, true, nil
}

func (f Files) Write(ctx context.Context, path string, content []byte) error {
	result, err := f.write(ctx, path, content, "create", "")
	if err == nil && result.Error != nil && result.Error.Code == "conflict" {
		// Replacing needs the current digest, so a concurrent change fails
		// instead of being overwritten.
		var current string
		if current, err = f.digest(ctx, path); err == nil {
			result, err = f.write(ctx, path, content, "replace", current)
		}
	}
	if err != nil {
		return err
	}
	if result.Error != nil {
		return fmt.Errorf("write %s: %s", path, result.Error.Message)
	}
	return nil
}

func (f Files) write(ctx context.Context, path string, content []byte, mode, expected string) (envelope, error) {
	arguments := map[string]any{"path": path, "content": string(content), "mode": mode}
	if expected != "" {
		arguments["expected_sha256"] = expected
	}
	return f.local(ctx, "write", arguments)
}

// digest is the SHA-256 of the whole file, which replace takes as its
// precondition; the runtime reports it only for a complete read.
func (f Files) digest(ctx context.Context, path string) (string, error) {
	content, found, err := f.Read(ctx, path, maxDigestRead)
	if err != nil {
		return "", err
	}
	if !found || len(content) >= maxDigestRead {
		return "", fmt.Errorf("read %s: cannot take its digest", path)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

// MakeDir creates the directory with mkdir -p; the write tool does not create
// parents.
func (f Files) MakeDir(ctx context.Context, path string) error {
	result, err := (Checks{Tools: f.Tools}).Run(ctx, domain.CheckCommand{Program: "mkdir", Args: []string{"-p", "--", path}})
	if err != nil {
		return err
	}
	if !result.Ran || result.ExitCode != 0 {
		return errors.New("mkdir -p " + path + ": " + result.Output)
	}
	return nil
}
