package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const maxCredentialsBytes = 16 << 10

// CredentialsStore reads and writes credentials.json.
type CredentialsStore struct{ Path string }

// Load returns the saved credentials; a missing file is no credentials.
func (s CredentialsStore) Load() (Credentials, error) {
	if !filepath.IsAbs(s.Path) {
		return Credentials{}, errors.New("credentials path must be absolute")
	}
	file, err := openNoFollow(s.Path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Credentials{}, err
	}
	if !privateRegular(info) {
		return Credentials{}, fmt.Errorf("%s holds a key and must be a regular file readable only by you; run chmod 600 %s", s.Path, s.Path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCredentialsBytes+1))
	if err != nil {
		return Credentials{}, err
	}
	if len(data) > maxCredentialsBytes {
		return Credentials{}, errors.New("credentials.json exceeds 16 KiB")
	}
	var credentials Credentials
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&credentials) != nil || decoder.Decode(new(any)) != io.EOF {
		return Credentials{}, errors.New("invalid credentials.json JSON")
	}
	if credentials.OpenRouterAPIKey != "" && !validAPIKey(credentials.OpenRouterAPIKey) {
		return Credentials{}, errors.New("credentials.json has an invalid openrouter_api_key")
	}
	if credentials.TypeSafeAPIKey != "" && !validAPIKey(credentials.TypeSafeAPIKey) {
		return Credentials{}, errors.New("credentials.json has an invalid typesafe_api_key")
	}
	return credentials, nil
}

// SaveOpenRouterKey writes the OpenRouter key, keeping the others.
func (s CredentialsStore) SaveOpenRouterKey(ctx context.Context, key string) error {
	return s.save(ctx, key, func(c *Credentials) { c.OpenRouterAPIKey = key })
}

// SaveTypeSafeKey writes Jev's key, keeping the others.
func (s CredentialsStore) SaveTypeSafeKey(ctx context.Context, key string) error {
	return s.save(ctx, key, func(c *Credentials) { c.TypeSafeAPIKey = key })
}

// save writes the file atomically with mode 0600 after applying change.
func (s CredentialsStore) save(ctx context.Context, key string, change func(*Credentials)) error {
	if !filepath.IsAbs(s.Path) {
		return errors.New("credentials path must be absolute")
	}
	if !validAPIKey(key) {
		return errors.New("the key is empty or contains spaces or control characters")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	credentials, err := s.Load()
	if err != nil {
		return err
	}
	change(&credentials)
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".axlr-credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), s.Path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(s.Path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return syncDirectoryFile(directory)
}

func validAPIKey(key string) bool {
	if key == "" || len(key) > 512 || strings.TrimSpace(key) != key {
		return false
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
