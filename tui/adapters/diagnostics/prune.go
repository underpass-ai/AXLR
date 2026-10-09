package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	traceFileName        = regexp.MustCompile(`^trace-[0-9]+-[0-9]+\.jsonl$`)
	payloadDirectoryName = regexp.MustCompile(`^.+\.jsonl\.payloads-[0-9]+$`)
	payloadCaptureName   = regexp.MustCompile(`^[0-9]{6,}-(?:request|response)\.(?:json|sse)$`)
)

// PruneDefault deletes what earlier launches left in the default diagnostics
// directory once it is older than retention: trace files named by
// OpenDefault and payload directories, each judged by its own modification
// time. Only names AXLR gives are considered, so a trace chosen with
// --trace-file keeps its file; links are never followed; a payload
// directory loses only its captures and is removed when nothing else is
// left in it. keep names the current launch's paths. A zero or negative
// retention deletes nothing. Errors are returned for the caller to ignore:
// pruning must not stop a launch.
func PruneDefault(getenv func(string) string, retention time.Duration, now time.Time, keep ...string) error {
	if retention <= 0 {
		return nil
	}
	dir, err := DefaultDirectory(getenv)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	kept := make(map[string]bool, len(keep))
	for _, path := range keep {
		if path != "" {
			kept[filepath.Clean(path)] = true
		}
	}
	cutoff := now.Add(-retention)
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		trace, payloads := traceFileName.MatchString(name), payloadDirectoryName.MatchString(name)
		if (!trace && !payloads) || kept[path] {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		switch {
		case trace && info.Mode().IsRegular():
			errs = append(errs, ignoreMissing(os.Remove(path)))
		case payloads && info.IsDir():
			errs = append(errs, prunePayloadDirectory(path))
		}
	}
	return errors.Join(errs...)
}

func prunePayloadDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return ignoreMissing(err)
	}
	var errs []error
	for _, entry := range entries {
		if entry.Type().IsRegular() && payloadCaptureName.MatchString(entry.Name()) {
			errs = append(errs, ignoreMissing(os.Remove(filepath.Join(path, entry.Name()))))
		}
	}
	if remaining, err := os.ReadDir(path); err == nil && len(remaining) == 0 {
		errs = append(errs, ignoreMissing(os.Remove(path)))
	}
	return errors.Join(errs...)
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
