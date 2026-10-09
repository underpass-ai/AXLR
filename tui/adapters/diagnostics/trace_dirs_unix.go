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

// secureTraceDirectories creates the diagnostic directory and checks every
// ancestor. Each refusal names the directory, its mode where that is the
// cause, and the command that fixes it: on 10 October 2026 "diagnostic
// directory is writable by another account" left the person to find which
// ancestor was meant and what to change.
func secureTraceDirectories(path, base string) error {
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), current), current)
	for index, part := range parts {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create diagnostic directory %s: %w", current, err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect diagnostic directory %s: %w", current, err)
		}
		isBase := index == len(parts)-3
		private := index >= len(parts)-2
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS exposes /var (and thus its temporary test directories)
			// through a root-owned link to /private/var. Resolve only trusted
			// system ancestors, never the state base or private AXLR paths.
			link, ok := info.Sys().(*syscall.Stat_t)
			if !ok || link.Uid != 0 || isBase || private {
				return fmt.Errorf("diagnostic directory %s is a symbolic link; it must be a real directory", current)
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return fmt.Errorf("resolve diagnostic directory %s: %w", current, err)
			}
			current = resolved
			info, err = os.Lstat(current)
			if err != nil {
				return fmt.Errorf("inspect diagnostic directory %s: %w", current, err)
			}
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("diagnostic directory %s is not a directory; move it away or set XDG_STATE_HOME to another directory", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("diagnostic directory %s has no owner this system reports", current)
		}
		if stat.Uid != uint32(os.Geteuid()) && stat.Uid != 0 {
			return fmt.Errorf("diagnostic directory %s belongs to uid %d, neither this account (uid %d) nor root; set XDG_STATE_HOME to a directory you own", current, stat.Uid, os.Geteuid())
		}
		if isBase && stat.Uid != uint32(os.Geteuid()) {
			return fmt.Errorf("diagnostic state directory %s belongs to uid %d, not this account (uid %d); set XDG_STATE_HOME to a directory you own", current, stat.Uid, os.Geteuid())
		}
		mode := info.Mode().Perm()
		if isBase && mode&0o022 != 0 {
			return fmt.Errorf("diagnostic state directory %s (mode %04o) is writable by another account; run chmod go-w %s", current, mode, current)
		}
		if mode&0o022 != 0 && (private || info.Mode()&os.ModeSticky == 0) {
			// AXLR's own directories must be 0700; an ancestor only has to
			// stop other accounts from replacing what lies under it.
			fix := "chmod go-w"
			if private {
				fix = "chmod 700"
			}
			return fmt.Errorf("diagnostic directory %s (mode %04o) is writable by another account; run %s %s", current, mode, fix, current)
		}
		if private {
			if stat.Uid != uint32(os.Geteuid()) {
				return fmt.Errorf("diagnostic directory %s belongs to uid %d, not this account (uid %d); remove it or set XDG_STATE_HOME to a directory you own", current, stat.Uid, os.Geteuid())
			}
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("secure diagnostic directory %s: %w", current, err)
			}
		}
	}
	return nil
}
