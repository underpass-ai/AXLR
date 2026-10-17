// Package registry is the console's client of a catalog's registry
// (axlr-registry docs/client-identity.md): the client key under the
// console's private state, and the HTTP client that signs every request
// with it.
package registry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/underpass-ai/AXLR/tui/application"
)

// KeyFileHeader is the first line of the key file: the format the
// axlr-registry CLI's publisher keygen writes too, so one key serves both
// clients. The second line is the base64 of the 32-byte seed.
const (
	KeyFileHeader = "axlr-registry publisher key v1"
	KeyFileName   = "client.key"
)

// KeyStore keeps the client key at Dir/client.key, mode 0600; it
// satisfies application.ClientKeyPort.
type KeyStore struct {
	Dir string
	mu  sync.Mutex
}

var _ application.ClientKeyPort = (*KeyStore)(nil)

// Path is the key file.
func (s *KeyStore) Path() string { return filepath.Join(s.Dir, KeyFileName) }

// Load implements application.ClientKeyPort.
func (s *KeyStore) Load(ctx context.Context) (*application.ClientKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(s.Dir) {
		return nil, errors.New("registry key directory must be absolute")
	}
	data, err := os.ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseKeyFile(s.Path(), data)
}

// ParseKeyFile reads a key file's two lines.
func ParseKeyFile(path string, data []byte) (*application.ClientKey, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != KeyFileHeader {
		return nil, fmt.Errorf("%s is not a client key file (expected the header %q)", path, KeyFileHeader)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s: the second line must be the base64 of 32 bytes", path)
	}
	return application.NewClientKey(ed25519.NewKeyFromSeed(seed))
}

// Create implements application.ClientKeyPort: a new key, written
// owner-only through a temporary file; an existing key is never replaced.
func (s *KeyStore) Create(ctx context.Context) (*application.ClientKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(s.Dir) {
		return nil, errors.New("registry key directory must be absolute")
	}
	if _, err := os.Lstat(s.Path()); err == nil {
		return nil, fmt.Errorf("%s exists; /tools identity shows it, and a new key means enrolling again, so move the file away first", s.Path())
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	key, err := application.NewClientKey(private)
	if err != nil {
		return nil, err
	}
	text := KeyFileHeader + "\n" + base64.StdEncoding.EncodeToString(key.Seed()) + "\n"
	file, err := os.CreateTemp(s.Dir, ".client-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := file.WriteString(text); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	// O_EXCL through a link: a key that appeared meanwhile is not replaced.
	if err := os.Link(file.Name(), s.Path()); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("%s exists; /tools identity shows it", s.Path())
		}
		return nil, err
	}
	return key, nil
}
