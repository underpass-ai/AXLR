package storage

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// InstallID is this install's stable id, kept in <dir>/install-id: 32 hex
// characters drawn once. It names the console's registration in the memory
// engine (axlr:<install-id>) and its host identity in the orchestrator.
func InstallID(dir string) (string, error) {
	path := filepath.Join(dir, "install-id")
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); isHexID(id) {
			return id, nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	// The id appears whole or not at all, and two consoles starting at once
	// agree on the first one linked: a torn or replaced id would register
	// the console again under another name.
	temporary, err := os.CreateTemp(dir, ".install-id-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(id + "\n"); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(temporary.Name(), 0o600); err != nil {
		return "", err
	}
	if err := os.Link(temporary.Name(), path); err != nil {
		if data, readErr := os.ReadFile(path); readErr == nil {
			if existing := strings.TrimSpace(string(data)); isHexID(existing) {
				return existing, nil
			}
		}
		// An unreadable or invalid file is replaced.
		if err := os.Rename(temporary.Name(), path); err != nil {
			return "", err
		}
	}
	return id, nil
}

func isHexID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
