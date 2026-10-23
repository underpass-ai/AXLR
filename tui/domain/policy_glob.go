package domain

// globMatch matches value against a pattern where * matches any run of
// characters but /, ** any run including /, and ? one character but /.
// Everything else matches itself. Patterns are short (PolicyRule limits
// them), so the backtracking is cheap.
func globMatch(pattern, value string) bool {
	if pattern == "" {
		return value == ""
	}
	switch {
	case len(pattern) >= 2 && pattern[0] == '*' && pattern[1] == '*':
		rest := pattern[2:]
		for i := 0; ; i++ {
			if globMatch(rest, value[i:]) {
				return true
			}
			if i >= len(value) {
				break
			}
		}
		// "a/**/b" also matches "a/b": the stars may stand for no segment.
		return len(rest) > 0 && rest[0] == '/' && globMatch(rest[1:], value)
	case pattern[0] == '*':
		for i := 0; ; i++ {
			if globMatch(pattern[1:], value[i:]) {
				return true
			}
			if i >= len(value) || value[i] == '/' {
				return false
			}
		}
	case pattern[0] == '?':
		return len(value) > 0 && value[0] != '/' && globMatch(pattern[1:], value[1:])
	}
	return len(value) > 0 && value[0] == pattern[0] && globMatch(pattern[1:], value[1:])
}
