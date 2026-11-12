package application

import "strings"

// MaxToolCallErrorBytes bounds the failure text a finished call carries
// to its readers: the app log and the exec events. Until 12 November 2026
// the exec event kept the first line and 240 characters, and a refused
// memory write reached the bot's panel as "…is a teammat…".
const MaxToolCallErrorBytes = 2 << 10

// ToolCallErrorText is a failed call's text on one line: each line
// trimmed, the empty ones dropped, the rest joined with " | ", at most
// limit bytes, cut on a character boundary with "…" when longer.
func ToolCallErrorText(content string, limit int) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	text := strings.Join(kept, " | ")
	if len(text) <= limit {
		return text
	}
	const ellipsis = "…"
	if limit <= len(ellipsis) {
		return utf8Prefix(text, limit)
	}
	return utf8Prefix(text, limit-len(ellipsis)) + ellipsis
}
