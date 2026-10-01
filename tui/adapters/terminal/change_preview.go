package terminal

import (
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type changeLine struct {
	Kind      udiff.OpKind
	Old, New  int
	Text      string
	Heading   bool
	NoNewline bool
	// OldCount and NewCount size a heading's hunk on each side.
	OldCount, NewCount int
}

type changePreview struct {
	Lines          []changeLine
	Added, Removed int
}

func previewChange(c domain.FileChange) changePreview {
	if c.Unavailable != "" || c.Validate() != nil {
		return changePreview{}
	}
	before, after := string(c.Before), string(c.After)
	diff, err := udiff.ToUnifiedDiff("", "", before, udiff.Lines(before, after), 3)
	if err != nil {
		return changePreview{}
	}
	var preview changePreview
	for _, hunk := range diff.Hunks {
		old, next := hunk.FromLine, hunk.ToLine
		heading := changeLine{Heading: true, Old: old, New: next}
		for _, line := range hunk.Lines {
			if line.Kind != udiff.Insert {
				heading.OldCount++
			}
			if line.Kind != udiff.Delete {
				heading.NewCount++
			}
		}
		preview.Lines = append(preview.Lines, heading)
		for _, line := range hunk.Lines {
			row := changeLine{Kind: line.Kind, Text: strings.TrimSuffix(line.Content, "\n")}
			switch line.Kind {
			case udiff.Delete:
				row.Old = old
				old++
				preview.Removed++
			case udiff.Insert:
				row.New = next
				next++
				preview.Added++
			default:
				row.Old, row.New = old, next
				old++
				next++
			}
			preview.Lines = append(preview.Lines, row)
			if !strings.HasSuffix(line.Content, "\n") {
				preview.Lines = append(preview.Lines, changeLine{NoNewline: true})
			}
		}
	}
	return preview
}

// render draws the diff. A new file gets one "new file" heading; other
// hunks name the lines they cover in the resulting file.
func (p changePreview) render(theme Theme, newFile bool) string {
	var lines []string
	for _, row := range p.Lines {
		if row.Heading {
			lines = append(lines, theme.reviewLine(hunkHeading(row, newFile, theme), theme.palette().Accent))
			continue
		}
		if row.NoNewline {
			lines = append(lines, theme.reviewLine("            \\ "+theme.T("changes.noNewline"), theme.palette().Muted))
			continue
		}
		old, next := "", ""
		if row.Old > 0 {
			old = fmt.Sprint(row.Old)
		}
		if row.New > 0 {
			next = fmt.Sprint(row.New)
		}
		prefix := " "
		if row.Kind == udiff.Delete {
			prefix = "-"
		} else if row.Kind == udiff.Insert {
			prefix = "+"
		}
		gutter := fmt.Sprintf("%4s %4s %s ", old, next, prefix)
		text := strings.ReplaceAll(Sanitize(row.Text), "\t", "    ")
		line := theme.reviewLine(gutter+text, theme.palette().Text)
		if row.Kind != udiff.Equal {
			line = theme.changeColor(gutter+text, row.Kind == udiff.Insert)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func hunkHeading(h changeLine, newFile bool, theme Theme) string {
	switch {
	case newFile:
		return theme.T("changes.hunkNew")
	case h.NewCount == 0:
		return theme.Tf("changes.hunkRemoved", h.Old, h.Old+h.OldCount-1)
	case h.NewCount == 1:
		return theme.Tf("changes.hunkLine", h.New)
	default:
		return theme.Tf("changes.hunkLines", h.New, h.New+h.NewCount-1)
	}
}
