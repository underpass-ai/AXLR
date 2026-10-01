package diagnostics

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// PayloadRecorder saves redacted bodies only, never authorization headers.
// Each individual capture is bounded; a long session retains every exchange.
type PayloadRecorder struct {
	directory string
	secrets   []string
	mu        sync.Mutex
}

const payloadLimit = 8 * 1024 * 1024

var credentialPattern = regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+|\bsk-[a-z0-9_-]{16,}`)

func NewPayloadRecorder(directory string, secrets ...string) (*PayloadRecorder, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("payload directory must be absolute")
	}
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !privatePayloadDirectory(info) {
		return nil, errors.New("payload directory must be a private directory")
	}
	return &PayloadRecorder{directory: directory, secrets: append([]string(nil), secrets...)}, nil
}
func (p *PayloadRecorder) Save(id uint64, kind string, body []byte) error {
	return p.save(id, kind, body, "sse")
}

// SaveResponse records the response's actual wire format.
func (p *PayloadRecorder) SaveResponse(id uint64, body []byte, sse bool) error {
	extension := "json"
	if sse {
		extension = "sse"
	}
	return p.save(id, "response", body, extension)
}

func (p *PayloadRecorder) save(id uint64, kind string, body []byte, responseExtension string) error {
	if kind != "request" && kind != "response" {
		return errors.New("invalid payload kind")
	}
	if len(body) > payloadLimit {
		return errors.New("payload exceeds capture limit")
	}
	body = append([]byte(nil), body...)
	for _, secret := range p.secrets {
		if secret != "" {
			body = bytes.ReplaceAll(body, []byte(secret), []byte("[redacted]"))
		}
	}
	// Both credential patterns begin with b/B or the Unicode case-fold set
	// s/S/ſ. Avoid the regex scan when none exists in a large payload.
	if bytes.IndexAny(body, "bBsS") >= 0 || bytes.Contains(body, []byte("ſ")) {
		body = credentialPattern.ReplaceAll(body, []byte("[redacted]"))
	}
	if len(body) > payloadLimit {
		return errors.New("redacted payload exceeds capture limit")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	extension := "json"
	if kind == "response" {
		extension = responseExtension
	}
	path := filepath.Join(p.directory, fmt.Sprintf("%06d-%s.%s", id, kind, extension))
	file, err := openDiagnosticFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
