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
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
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
