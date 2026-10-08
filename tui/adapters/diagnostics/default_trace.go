package diagnostics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// OpenDefault gives every launch its own trace. Environment lookup is injected
// so callers and tests never fall back to another account's home directory.
func OpenDefault(getenv func(string) string) (*FileLogger, string, error) {
	dir, err := DefaultDirectory(getenv)
	if err != nil {
		return nil, "", err
	}
	file, err := os.CreateTemp(dir, fmt.Sprintf("trace-%d-*.jsonl", os.Getpid()))
	if err != nil {
		return nil, "", fmt.Errorf("create default diagnostic log: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, "", fmt.Errorf("close default diagnostic log: %w", err)
	}
	logger, err := Open(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, "", err
	}
	return logger, path, nil
}

// DefaultDirectory is the private axlr/logs directory under the state base,
// created if missing.
func DefaultDirectory(getenv func(string) string) (string, error) {
	if getenv == nil {
		return "", errors.New("diagnostic environment is unavailable")
	}
	base := getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home := getenv("HOME")
		if runtime.GOOS == "windows" && !filepath.IsAbs(home) {
			home = getenv("USERPROFILE")
		}
		if !filepath.IsAbs(home) {
			return "", errors.New("absolute HOME or XDG_STATE_HOME is required for diagnostics")
		}
		if runtime.GOOS == "windows" {
			base = filepath.Join(home, "AppData", "Local")
		} else {
			base = filepath.Join(home, ".local", "state")
		}
	}
	dir := filepath.Join(base, "axlr", "logs")
	if err := secureTraceDirectories(dir, filepath.Clean(base)); err != nil {
		return "", err
	}
	return dir, nil
}
