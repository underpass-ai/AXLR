package diagnostics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// OpenDefault gives every launch its own trace. Environment lookup is injected
// so callers and tests never fall back to another account's home directory.
func OpenDefault(getenv func(string) string) (*FileLogger, string, error) {
	if getenv == nil {
		return nil, "", errors.New("diagnostic environment is unavailable")
	}
	base := getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home := getenv("HOME")
		if !filepath.IsAbs(home) {
			return nil, "", errors.New("absolute HOME or XDG_STATE_HOME is required for diagnostics")
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "axlr", "logs")
	if err := secureTraceDirectories(dir, filepath.Clean(base)); err != nil {
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

func secureTraceDirectories(path, base string) error {
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), current), current)
	for index, part := range parts {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create diagnostic directory: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect diagnostic directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("diagnostic directory must be a real directory")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (stat.Uid != uint32(os.Geteuid()) && stat.Uid != 0) {
			return errors.New("diagnostic directory has an untrusted owner")
		}
		if current == base && (stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o022 != 0) {
			return errors.New("diagnostic state directory must belong to this account and reject other writers")
		}
		// A system temporary directory is a safe ancestor only with its sticky
		// bit. The two directories owned by AXLR must always be private.
		private := index >= len(parts)-2
		if info.Mode().Perm()&0o022 != 0 && (private || info.Mode()&os.ModeSticky == 0) {
			return errors.New("diagnostic directory is writable by another account")
		}
		if private {
			if stat.Uid != uint32(os.Geteuid()) {
				return errors.New("diagnostic directory must belong to this account")
			}
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("secure diagnostic directory: %w", err)
			}
		}
	}
	return nil
}
