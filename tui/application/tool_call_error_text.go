package application

import "strings"

// MaxToolCallErrorBytes bounds the failure text a finished call carries
// to its readers: the app log and the exec events. Until 12 November 2026
// the exec event kept the first line and 240 characters, and a refused
// memory write reached the bot's panel as "…is a teammat…".
const MaxToolCallErrorBytes = 2 << 10

// ToolCallErrorText is a failed call's text on one line: each line
// trimmed, the empty ones dropped, the rest joined with " | ", at most
// limit bytes. A longer text keeps its beginning and its end, cut on
// character boundaries, with " … " between: the actionable part of a
// refusal is often its end, and so is a failed command's standard error
// in the runtime's answer.
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
	const gap = " … "
	if limit <= 2*len(gap) {
		return utf8Prefix(text, limit)
	}
	head := utf8Prefix(text, (limit-len(gap))/2)
	return head + gap + utf8Suffix(text, limit-len(gap)-len(head))
}
