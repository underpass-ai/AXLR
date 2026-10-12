package terminal

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxWorkspaceFiles bounds the @ menu's walk of the workspace, as a
// local_list listing is bounded.
const maxWorkspaceFiles = 20000

// skippedDirectories are never listed for @: version control, dependencies
// and build output, which drown the files a prompt names.
var skippedDirectories = map[string]bool{".git": true, ".hg": true, ".svn": true, "node_modules": true, "vendor": true, "dist": true, "target": true, ".venv": true, "venv": true, "__pycache__": true, ".cache": true, ".idea": true, ".axlr": true}

// errWalkLimit stops the walk at maxWorkspaceFiles entries.
var errWalkLimit = errors.New("workspace walk limit")

// listWorkspaceFiles walks root depth first without following links and
// returns its files and directories (with a trailing "/") as slash paths
// relative to it, leaving out skippedDirectories and what the simple
// patterns of the root .gitignore match; truncated reports a walk that
// reached maxWorkspaceFiles.
func listWorkspaceFiles(root string) (files []string, truncated bool, err error) {
	ignore := readGitignore(filepath.Join(root, ".gitignore"))
	err = filepath.WalkDir(root, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if full == root {
				return walkErr
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if full == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		dir := entry.IsDir()
		if dir && skippedDirectories[entry.Name()] || ignore.matches(rel, dir) {
			if dir {
				return fs.SkipDir
			}
			return nil
		}
		if len(files) >= maxWorkspaceFiles {
			truncated = true
			return errWalkLimit
		}
		if dir {
			rel += "/"
		}
		files = append(files, rel)
		return nil
	})
	if errors.Is(err, errWalkLimit) {
		err = nil
	}
	return files, truncated, err
}

// gitignore holds the root .gitignore's simple patterns: a name or glob
// matched against every path's last element, or, when it holds a slash,
// against the path from the root. Negations and "**" are not read.
type gitignore []struct {
	pattern   string
	anchored  bool
	directory bool
}

func readGitignore(name string) gitignore {
	file, err := os.Open(name)
	if err != nil {
		return nil
	}
	defer file.Close()
	var out gitignore
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") || strings.Contains(line, "**") {
			continue
		}
		directory := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		anchored := strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		if _, err := path.Match(line, ""); err != nil {
			continue
		}
		out = append(out, struct {
			pattern   string
			anchored  bool
			directory bool
		}{line, anchored, directory})
	}
	return out
}

func (g gitignore) matches(rel string, dir bool) bool {
	for _, p := range g {
		if p.directory && !dir {
			continue
		}
		subject := path.Base(rel)
		if p.anchored {
			subject = rel
		}
		if ok, _ := path.Match(p.pattern, subject); ok {
			return true
		}
	}
	return false
}

// rankFiles orders the files that match query: the name starting with it,
// the path starting with it, the name containing it, the path containing
// it, then its letters in order; shorter paths first within each.
func rankFiles(files []string, query string, limit int) []string {
	query = strings.ToLower(query)
	type ranked struct {
		path string
		rank int
	}
	var out []ranked
	for _, file := range files {
		lower := strings.ToLower(file)
		base := path.Base(strings.TrimSuffix(lower, "/"))
		rank := -1
		switch {
		case query == "":
			rank = strings.Count(strings.TrimSuffix(file, "/"), "/")
		case strings.HasPrefix(base, query):
			rank = 0
		case strings.HasPrefix(lower, query):
			rank = 1
		case strings.Contains(base, query):
			rank = 2
		case strings.Contains(lower, query):
			rank = 3
		case subsequence(lower, query):
			rank = 4
		}
		if rank >= 0 {
			out = append(out, ranked{file, rank})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		if len(out[i].path) != len(out[j].path) {
			return len(out[i].path) < len(out[j].path)
		}
		return out[i].path < out[j].path
	})
	paths := make([]string, 0, min(limit, len(out)))
	for _, r := range out[:min(limit, len(out))] {
		paths = append(paths, r.path)
	}
	return paths
}

// subsequence reports whether every rune of query appears in text in
// order.
func subsequence(text, query string) bool {
	runes := []rune(query)
	i := 0
	for _, r := range text {
		if i < len(runes) && r == runes[i] {
			i++
		}
	}
	return i == len(runes)
}
