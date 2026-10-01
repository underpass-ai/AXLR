package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const markdownReply = "Hay **18 ceremonias** publicadas.\n\n## Ceremonias de trabajo AXLR\n\n| Ceremonia | Para qué |\n|:--|:--|\n| `axlr_change` | Edición pequeña |\n| `axlr_delivery` | Build → verificación |\n\n- **`aeo_observatory_run`** 1.0\n> nota\n\n```go\nfmt.Println(\"**no**\")\n```\n---"

func TestRenderMarkdownDropsMarkersAndAlignsTables(t *testing.T) {
	got := renderMarkdown(markdownReply, Theme{Monochrome: true})
	want := strings.Join([]string{
		"Hay 18 ceremonias publicadas.",
		"",
		"Ceremonias de trabajo AXLR",
		"",
		"Ceremonia      Para qué",
		"─────────────  ────────────────────",
		"axlr_change    Edición pequeña",
		"axlr_delivery  Build → verificación",
		"",
		"• aeo_observatory_run 1.0",
		"▍ nota",
		"",
		"",
		`  fmt.Println("**no**")`,
		"",
		strings.Repeat("─", 24),
	}, "\n")
	if got != want {
		t.Fatalf("rendered markdown:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderMarkdownKeepsLineStructureAcrossThemes(t *testing.T) {
	plain := renderMarkdown(markdownReply, Theme{Monochrome: true})
	styled := renderMarkdown(markdownReply, Theme{ID: domain.ThemeInk})
	if styled == plain {
		t.Fatal("styled theme produced no styling")
	}
	if ansi.Strip(styled) != plain {
		t.Fatalf("styling changed the text:\n%s\nwant:\n%s", ansi.Strip(styled), plain)
	}
}

func TestRenderMarkdownLeavesUnclosedMarkersLiteral(t *testing.T) {
	for _, partial := range []string{"a **half", "a `half", "```go\nstill code", "| a | b |\n|:-"} {
		got := ansi.Strip(renderMarkdown(partial, Theme{ID: domain.ThemeInk}))
		if partial == "a **half" && got != "a **half" || partial == "a `half" && got != "a `half" {
			t.Fatalf("unclosed marker changed: %q -> %q", partial, got)
		}
		if strings.Count(got, "\n") != strings.Count(partial, "\n") {
			t.Fatalf("partial %q changed line count: %q", partial, got)
		}
	}
}

// Inline styles must hand the line back to the row's text colour, never to
// the terminal default, and must not reset the row style before its end.
func TestMarkdownStylesRestoreTheRowForeground(t *testing.T) {
	theme := Theme{ID: domain.ThemeInk}
	line := renderMarkdown("pre **bold** `code` tail", theme)
	row := theme.rowText(transcriptRowAssistant).Width(60).Render(line)
	before := row[:strings.Index(row, "tail")]
	if strings.Contains(before, "\x1b[0m") || strings.Contains(before, "\x1b[m") || strings.Contains(before, "\x1b[39m") {
		t.Fatalf("inline style reset the row before the tail: %q", row)
	}
	if !strings.Contains(before, sgrForeground(theme.palette().Text)) {
		t.Fatalf("row foreground not restored after code: %q", row)
	}
}

func TestTranscriptRendersAssistantMarkdownButNotToolRows(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("**literal** question", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "## Title\n**bold** answer"}}); err != nil {
		t.Fatal(err)
	}
	transcript := NewTranscript()
	transcript.SetWidth(60)
	transcript.SetSession(s.Export(), "", Theme{Monochrome: true})
	text := transcript.Text()
	if !strings.Contains(text, "\n\nTitle\nbold answer") {
		t.Fatalf("assistant markdown not rendered: %q", text)
	}
	if !strings.Contains(text, "> **literal** question") {
		t.Fatalf("user text should stay verbatim: %q", text)
	}
}

func TestWideTablesStackOneRowPerLineWhenTheyDoNotFit(t *testing.T) {
	table := "| Ceremonia | Para qué |\n|:--|:--|\n| `axlr_change` | Edición pequeña, entendida y reversible |\n| `axlr_delivery` | Feature o refactor no trivial con build → verificación → revisión |"
	wide := renderMarkdownWidth(table, Theme{Monochrome: true}, 120)
	if !strings.Contains(wide, "Ceremonia      Para qué") {
		t.Fatalf("a table that fits lost its columns:\n%s", wide)
	}
	narrow := renderMarkdownWidth(table, Theme{Monochrome: true}, 50)
	want := "axlr_change — Edición pequeña, entendida y reversible\naxlr_delivery — Feature o refactor no trivial con build → verificación → revisión"
	if narrow != want {
		t.Fatalf("narrow table:\n%s\nwant:\n%s", narrow, want)
	}
	if ansi.Strip(renderMarkdownWidth(table, Theme{ID: domain.ThemeInk}, 50)) != narrow {
		t.Fatal("styling changed the stacked layout")
	}
}

func TestTranscriptRelaysTablesWhenTheWidthChanges(t *testing.T) {
	s := domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: root.Text("| A | B |\n|--|--|\n| " + strings.Repeat("x", 30) + " | " + strings.Repeat("y", 30) + " |")}}}
	tr := NewTranscript()
	tr.SetWidth(100)
	tr.SetSession(s, "", Theme{Monochrome: true})
	if !strings.Contains(tr.Text(), "A  ") {
		t.Fatalf("wide viewport should keep columns: %q", tr.Text())
	}
	tr.SetWidth(40)
	if !strings.Contains(tr.Text(), strings.Repeat("x", 30)+" — ") {
		t.Fatalf("narrow viewport should stack the table: %q", tr.Text())
	}
}
