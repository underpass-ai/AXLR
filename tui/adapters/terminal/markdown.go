package terminal

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var (
	markdownHeading   = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	markdownBullet    = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	markdownRule      = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	markdownTableRule = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?$`)
)

// markdownStyle holds the SGR codes for the markdown subset. It switches
// attributes off with targeted codes (22, or the row's text colour) instead of
// a full reset, so the row band behind the line is never cut short. A
// monochrome theme leaves every code empty: styling changes, structure never
// does, because the search jump measures a monochrome copy of the transcript.
type markdownStyle struct {
	boldOn, boldOff, accent, code, muted, text string
	// width is the columns a table may use; 0 means unbounded.
	width int
}

func newMarkdownStyle(theme Theme) markdownStyle {
	if theme.Monochrome {
		return markdownStyle{}
	}
	p := theme.palette()
	return markdownStyle{boldOn: "\x1b[1m", boldOff: "\x1b[22m", accent: sgrForeground(p.Accent), code: sgrForeground(p.Warning), muted: sgrForeground(p.Muted), text: sgrForeground(p.Text)}
}

func sgrForeground(hex string) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil || len(hex) != 7 {
		return ""
	}
	return "\x1b[38;2;" + strconv.Itoa(int(v>>16&0xff)) + ";" + strconv.Itoa(int(v>>8&0xff)) + ";" + strconv.Itoa(int(v&0xff)) + "m"
}

func (s markdownStyle) colour(code, text string) string {
	if code == "" {
		return text
	}
	return code + text + s.text
}

// renderMarkdown renders the markdown subset models use in replies: headings,
// bold, inline code, fenced code, bullets, quotes, rules and tables. Input is
// sanitized text; unclosed markers stay literal so streamed drafts render
// safely while they are incomplete.
func renderMarkdown(src string, theme Theme) string {
	return renderMarkdownWidth(src, theme, 0)
}

// renderMarkdownWidth renders like renderMarkdown, laying tables that do not
// fit the width out one row per line instead of in broken columns.
func renderMarkdownWidth(src string, theme Theme, width int) string {
	s := newMarkdownStyle(theme)
	s.width = width
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	fenced := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			fenced = !fenced
			out = append(out, "")
		case fenced:
			out = append(out, "  "+s.colour(s.code, line))
		case strings.HasPrefix(trimmed, "|"):
			end := i
			for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
				end++
			}
			out = append(out, s.table(lines[i:end])...)
			i = end - 1
		case markdownHeading.MatchString(line):
			text := strings.ReplaceAll(markdownHeading.FindStringSubmatch(line)[1], "**", "")
			out = append(out, s.boldOn+s.colour(s.accent, s.codeSpans(text))+s.boldOff)
		case markdownRule.MatchString(line):
			out = append(out, s.colour(s.muted, strings.Repeat("─", 24)))
		case markdownBullet.MatchString(line):
			m := markdownBullet.FindStringSubmatch(line)
			out = append(out, m[1]+s.colour(s.muted, "•")+" "+s.inline(m[2]))
		case strings.HasPrefix(line, ">"):
			out = append(out, s.colour(s.muted, "▍")+" "+s.inline(strings.TrimPrefix(strings.TrimPrefix(line, ">"), " ")))
		default:
			out = append(out, s.inline(line))
		}
	}
	return strings.Join(out, "\n")
}

// inline renders code spans and bold, whichever opens first. Code spans keep
// their contents literal; bold may contain code spans.
func (s markdownStyle) inline(text string) string {
	return s.spans(text, true)
}

func (s markdownStyle) codeSpans(text string) string {
	return s.spans(strings.ReplaceAll(text, "**", ""), false)
}

func (s markdownStyle) spans(text string, bold bool) string {
	var b strings.Builder
	for {
		code := strings.IndexByte(text, '`')
		strong := -1
		if bold {
			strong = strings.Index(text, "**")
		}
		if code >= 0 && (strong < 0 || code < strong) {
			if close := strings.IndexByte(text[code+1:], '`'); close >= 0 {
				b.WriteString(text[:code])
				b.WriteString(s.colour(s.code, text[code+1:code+1+close]))
				text = text[code+2+close:]
				continue
			}
			code = -1
		}
		if strong >= 0 {
			if close := strings.Index(text[strong+2:], "**"); close >= 0 {
				b.WriteString(text[:strong])
				b.WriteString(s.boldOn + s.spans(text[strong+2:strong+2+close], false) + s.boldOff)
				text = text[strong+4+close:]
				continue
			}
		}
		if code < 0 && strong >= 0 {
			// An unclosed ** still allows code spans after it.
			b.WriteString(text[:strong+2])
			text = text[strong+2:]
			bold = false
			continue
		}
		break
	}
	b.WriteString(text)
	return b.String()
}

// table aligns pipe-table rows by display width. The delimiter row becomes a
// single rule; rows wider than the viewport wrap like any other line.
func (s markdownStyle) table(rows []string) []string {
	cells := make([][]string, 0, len(rows))
	ruleAt := -1
	columns := 0
	for _, row := range rows {
		trimmed := strings.TrimSpace(row)
		if markdownTableRule.MatchString(trimmed) && strings.Contains(trimmed, "-") {
			ruleAt = len(cells)
			cells = append(cells, nil)
			continue
		}
		parts := strings.Split(strings.Trim(trimmed, "|"), "|")
		rendered := make([]string, len(parts))
		for i, part := range parts {
			text := strings.TrimSpace(part)
			if ruleAt < 0 && len(cells) == 0 {
				rendered[i] = s.boldOn + s.codeSpans(text) + s.boldOff
			} else {
				rendered[i] = s.inline(text)
			}
		}
		columns = max(columns, len(rendered))
		cells = append(cells, rendered)
	}
	widths := make([]int, columns)
	for _, row := range cells {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}
	total := 2 * (columns - 1)
	for _, w := range widths {
		total += w
	}
	if s.width > 0 && total > s.width {
		return s.stackedTable(cells, ruleAt)
	}
	out := make([]string, 0, len(cells))
	for index, row := range cells {
		var b strings.Builder
		for i := range columns {
			if i > 0 {
				b.WriteString("  ")
			}
			if index == ruleAt {
				b.WriteString(s.colour(s.muted, strings.Repeat("─", widths[i])))
				continue
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(cell)
			if i < columns-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-ansi.StringWidth(cell)))
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

// stackedTable writes each data row as "first — second · third": readable at
// any width, unlike columns that wrap into each other.
func (s markdownStyle) stackedTable(cells [][]string, ruleAt int) []string {
	var out []string
	for index, row := range cells {
		if index == ruleAt || len(row) == 0 || ruleAt > 0 && index < ruleAt && len(cells) > ruleAt+1 {
			continue
		}
		line := row[0]
		var rest []string
		for _, cell := range row[1:] {
			if ansi.Strip(cell) != "" {
				rest = append(rest, cell)
			}
		}
		if len(rest) > 0 {
			line += s.colour(s.muted, " — ") + strings.Join(rest, s.colour(s.muted, " · "))
		}
		out = append(out, line)
	}
	return out
}
