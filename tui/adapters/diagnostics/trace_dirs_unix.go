//go:build !windows

package diagnostics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

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
		isBase := index == len(parts)-3
		private := index >= len(parts)-2
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS exposes /var (and thus its temporary test directories)
			// through a root-owned link to /private/var. Resolve only trusted
			// system ancestors, never the state base or private AXLR paths.
			link, ok := info.Sys().(*syscall.Stat_t)
			if !ok || link.Uid != 0 || isBase || private {
				return errors.New("diagnostic directory must be a real directory")
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return err
			}
			current = resolved
			info, err = os.Lstat(current)
			if err != nil {
				return err
			}
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("diagnostic directory must be a real directory")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (stat.Uid != uint32(os.Geteuid()) && stat.Uid != 0) {
			return errors.New("diagnostic directory has an untrusted owner")
		}
		if isBase && (stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o022 != 0) {
			return errors.New("diagnostic state directory must belong to this account and reject other writers")
		}
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
