package domain

import (
	"errors"
	"regexp"
	"strings"
)

// maxSearchPatternBytes bounds a pattern; RE2 matching stays linear in the
// text, but a pattern this long is a mistake rather than a search.
const maxSearchPatternBytes = 4096

// SearchPattern is a compiled RE2 expression matched against one line at a
// time, so ^ and $ anchor to the line.
type SearchPattern struct{ expression *regexp.Regexp }

// NewSearchPattern compiles pattern as Go's RE2 syntax, or as literal text
// when literal is set; ignoreCase makes either case-insensitive.
func NewSearchPattern(pattern string, literal, ignoreCase bool) (SearchPattern, error) {
	if pattern == "" {
		return SearchPattern{}, errors.New("pattern is required")
	}
	if len(pattern) > maxSearchPatternBytes || strings.ContainsRune(pattern, '\x00') {
		return SearchPattern{}, errors.New("pattern exceeds 4096 bytes or contains NUL")
	}
	if literal {
		pattern = regexp.QuoteMeta(pattern)
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return SearchPattern{}, errors.New("invalid pattern: " + err.Error())
	}
	return SearchPattern{expression: expression}, nil
}

// Find returns the byte index of the first match in line.
func (p SearchPattern) Find(line string) (int, bool) {
	if p.expression == nil {
		return 0, false
	}
	at := p.expression.FindStringIndex(line)
	if at == nil {
		return 0, false
	}
	return at[0], true
}
