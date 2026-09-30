//go:build windows

package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func secureTraceDirectories(path, base string) error {
	if !filepath.IsAbs(path) || !filepath.IsAbs(base) {
		return errors.New("diagnostic paths must be absolute")
	}
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(clean, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("diagnostic directory must be a real directory")
		}
	}
	return nil
}
