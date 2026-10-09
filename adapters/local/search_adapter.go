package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

const (
	// searchFileBytes is the largest file a search reads; larger files are
	// counted as skipped, like binary files and invalid UTF-8.
	searchFileBytes = 1 << 20
	// binarySniffBytes is how much of a file is checked for NUL, as ripgrep
	// and git do.
	binarySniffBytes = 8 << 10
	// lineBytes bounds a reported line; lineLead is how much of the line a
	// window keeps before a match that starts past the bound.
	lineBytes = 300
	lineLead  = 64
	ellipsis  = "…"
)

// errWalkStop ends a walk early without an error.
var errWalkStop = errors.New("walk stopped")

// Search walks c.Scope through the workspace root in name order, depth
// first, and reports the lines matching c.Pattern. Every entry is Lstat'ed
// through the root: a symlink is never followed, .git is never entered and
// only regular files are opened, so nothing outside the workspace is read.
func (a *FileAdapter) Search(ctx context.Context, c domain.SearchCommand) (domain.SearchResult, error) {
	result := domain.SearchResult{Matches: []domain.SearchMatch{}, Offset: c.Offset}
	seen := 0
	var scanned int64
	visit := func(rel string, info fs.FileInfo, filtered bool) error {
		if err := ctx.Err(); err != nil {
			return cancelled()
		}
		if filtered && !c.Glob.Matches(rel) {
			return nil
		}
		if result.FilesScanned+result.FilesSkipped >= c.MaxFiles {
			result.LimitReached = true
			return errWalkStop
		}
		if info.Size() > searchFileBytes {
			result.FilesSkipped++
			return nil
		}
		if scanned+info.Size() > c.MaxScanBytes {
			result.LimitReached = true
			return errWalkStop
		}
		content, ok := a.readText(rel)
		if !ok {
			result.FilesSkipped++
			return nil
		}
		scanned += int64(len(content))
		result.FilesScanned++
		lines := strings.Split(content, "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for i, line := range lines {
			line = strings.TrimSuffix(line, "\r")
			at, found := c.Pattern.Find(line)
			if !found {
				continue
			}
			seen++
			if seen <= c.Offset {
				continue
			}
			if len(result.Matches) == c.MaxResults {
				result.More = true
				return errWalkStop
			}
			match := domain.SearchMatch{Path: rel, Line: i + 1, Column: at + 1}
			var cut bool
			match.Text, cut = excerptLine(line, at)
			match.Truncated = cut
			for j := max(0, i-c.ContextLines); j < i; j++ {
				text, cut := excerptLine(strings.TrimSuffix(lines[j], "\r"), 0)
				match.Before = append(match.Before, text)
				match.Truncated = match.Truncated || cut
			}
			for j := i + 1; j <= min(len(lines)-1, i+c.ContextLines); j++ {
				text, cut := excerptLine(strings.TrimSuffix(lines[j], "\r"), 0)
				match.After = append(match.After, text)
				match.Truncated = match.Truncated || cut
			}
			result.Matches = append(result.Matches, match)
		}
		return nil
	}
	info, err := a.root.Lstat(string(c.Scope))
	if err != nil {
		return domain.SearchResult{}, fileError(err)
	}
	switch {
	case info.IsDir():
		err = a.walk(string(c.Scope), 1, 0, func(rel string, info fs.FileInfo, _ int) (bool, error) {
			if info.Name() == ".git" {
				return false, nil
			}
			if info.IsDir() {
				return true, ctx.Err()
			}
			if !info.Mode().IsRegular() {
				return false, nil
			}
			return false, visit(rel, info, true)
		}, func() { result.FilesSkipped++ })
	case info.Mode().IsRegular():
		// A file named by the caller is searched whatever the glob says.
		err = visit(string(c.Scope), info, false)
	default:
		return domain.SearchResult{}, domain.Reject("not_searchable", "search path must be a directory or a regular file; symlinks are not followed")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		err = cancelled()
	}
	if err != nil && !errors.Is(err, errWalkStop) {
		return domain.SearchResult{}, err
	}
	return result, nil
}

// List reports the entries under c.Scope in name order, depth first, down
// to c.Depth levels. A symlink is listed as one and never followed; .git is
// listed but not entered. A scope that is not a directory lists itself.
func (a *FileAdapter) List(ctx context.Context, c domain.ListCommand) (domain.ListResult, error) {
	result := domain.ListResult{Entries: []domain.ListEntry{}, Offset: c.Offset}
	info, err := a.root.Lstat(string(c.Scope))
	if err != nil {
		return domain.ListResult{}, fileError(err)
	}
	if !info.IsDir() {
		if c.Offset == 0 {
			result.Entries = append(result.Entries, listEntry(string(c.Scope), info))
		}
		return result, nil
	}
	seen, visits := 0, 0
	err = a.walk(string(c.Scope), 1, c.Depth, func(rel string, info fs.FileInfo, _ int) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, cancelled()
		}
		visits++
		if visits > c.MaxVisits {
			result.LimitReached = true
			return false, errWalkStop
		}
		if c.Glob.Matches(rel) {
			seen++
			if seen > c.Offset {
				if len(result.Entries) == c.MaxEntries {
					result.More = true
					return false, errWalkStop
				}
				result.Entries = append(result.Entries, listEntry(rel, info))
			}
		}
		return info.IsDir() && info.Name() != ".git", nil
	}, func() {})
	if err != nil && !errors.Is(err, errWalkStop) {
		return domain.ListResult{}, err
	}
	return result, nil
}

// walk calls visit for each entry of dir, in byte order of name, and enters
// the directories visit asks for while depth is below maxDepth (0 for no
// bound). Entries are Lstat'ed through the root, never through a DirEntry,
// whose Info would resolve the real path outside it. A subdirectory that
// cannot be read is reported to unreadable and passed over; the scope itself
// failing is an error.
func (a *FileAdapter) walk(dir string, depth, maxDepth int, visit func(rel string, info fs.FileInfo, depth int) (bool, error), unreadable func()) error {
	names, err := a.names(dir)
	if err != nil {
		return fileError(err)
	}
	return a.walkNames(dir, names, depth, maxDepth, visit, unreadable)
}

func (a *FileAdapter) walkNames(dir string, names []string, depth, maxDepth int, visit func(string, fs.FileInfo, int) (bool, error), unreadable func()) error {
	for _, name := range names {
		rel := name
		if dir != "." {
			rel = dir + "/" + name
		}
		info, err := a.root.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed during the walk
		}
		if err != nil {
			unreadable()
			continue
		}
		enter, err := visit(rel, info, depth)
		if err != nil {
			return err
		}
		if !enter || !info.IsDir() || (maxDepth > 0 && depth >= maxDepth) {
			continue
		}
		children, err := a.names(rel)
		if err != nil {
			unreadable()
			continue
		}
		if err := a.walkNames(rel, children, depth+1, maxDepth, visit, unreadable); err != nil {
			return err
		}
	}
	return nil
}

// names lists a directory through the root, sorted, since ReadDir returns
// entries in directory order.
func (a *FileAdapter) names(dir string) ([]string, error) {
	f, err := a.root.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// readText returns a file's text, or false when it cannot be opened, is no
// longer a regular file, exceeds searchFileBytes, has a NUL in its first
// 8 KiB or is not UTF-8.
func (a *FileAdapter) readText(rel string) (string, bool) {
	f, err := a.root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", false
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(f, searchFileBytes+1))
	if err != nil || len(b) > searchFileBytes || bytes.IndexByte(b[:min(len(b), binarySniffBytes)], 0) >= 0 || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}

// excerptLine bounds line to lineBytes at character boundaries. A match at
// byte at that would fall past the bound gets a window that starts lineLead
// bytes before it. Each cut end is marked with an ellipsis.
func excerptLine(line string, at int) (string, bool) {
	if len(line) <= lineBytes {
		return line, false
	}
	start := 0
	if at > lineBytes-lineLead {
		start = at - lineLead
		for start < len(line) && !utf8.RuneStart(line[start]) {
			start++
		}
	}
	end := len(line)
	if start+lineBytes < end {
		end = start + lineBytes
		for end > start && !utf8.RuneStart(line[end]) {
			end--
		}
	}
	text := line[start:end]
	if start > 0 {
		text = ellipsis + text
	}
	if end < len(line) {
		text += ellipsis
	}
	return text, true
}

func listEntry(rel string, info fs.FileInfo) domain.ListEntry {
	entry := domain.ListEntry{Path: rel, Type: domain.EntryOther}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		entry.Type = domain.EntrySymlink
	case info.IsDir():
		entry.Type = domain.EntryDir
	case info.Mode().IsRegular():
		entry.Type, entry.Size = domain.EntryFile, info.Size()
	}
	return entry
}

func cancelled() error {
	return &domain.Fault{Status: "cancelled", Code: "cancelled", Message: "request cancelled; nothing was changed"}
}
