package domain

import (
	"errors"
	"path"
	"strings"
)

// NameGlob filters searched files and listed entries with path.Match. A
// pattern without a slash ("*.go") matches the base name at any depth; one
// with a slash ("tui/*.go") matches the whole workspace-relative path; a
// leading "**/" lets the rest match at any directory ("**/cmd/*.go"). The
// empty glob matches everything.
type NameGlob struct {
	pattern  string
	anywhere bool
}

func NewNameGlob(s string) (NameGlob, error) {
	pattern, anywhere := strings.CutPrefix(s, "**/")
	if s == "" {
		return NameGlob{}, nil
	}
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.ContainsRune(pattern, '\x00') {
		return NameGlob{}, errors.New("glob must be a relative pattern such as *.go")
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return NameGlob{}, errors.New("invalid glob: " + err.Error())
	}
	return NameGlob{pattern: pattern, anywhere: anywhere}, nil
}

// Matches reports whether the slash-separated workspace path rel passes.
func (g NameGlob) Matches(rel string) bool {
	if g.pattern == "" {
		return true
	}
	if !strings.Contains(g.pattern, "/") {
		ok, _ := path.Match(g.pattern, path.Base(rel))
		return ok
	}
	if ok, _ := path.Match(g.pattern, rel); ok || !g.anywhere {
		return ok
	}
	for i := 0; i < len(rel); i++ {
		if rel[i] == '/' {
			if ok, _ := path.Match(g.pattern, rel[i+1:]); ok {
				return true
			}
		}
	}
	return false
}
