package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestTranscriptWrapsAtWordBoundaries(t *testing.T) {
	transcript := NewTranscript()
	transcript.SetWidth(20)
	transcript.SetContent("obtuve las tres páginas hasta el final")
	for _, line := range strings.Split(transcript.Viewport.GetContent(), "\n") {
		if ansi.StringWidth(line) > 20 {
			t.Fatalf("line wider than viewport: %q", line)
		}
	}
	for _, word := range []string{"obtuve", "páginas", "hasta", "final"} {
		if !strings.Contains(transcript.Viewport.GetContent(), word) {
			t.Fatalf("word %q split across lines: %q", word, transcript.Viewport.GetContent())
		}
	}
}

func TestTranscriptRewrapsWhenWidthChanges(t *testing.T) {
	transcript := NewTranscript()
	transcript.SetWidth(10)
	transcript.SetContent("uno dos tres cuatro cinco")
	narrow := transcript.VisualLineCount()
	transcript.SetWidth(80)
	if wide := transcript.VisualLineCount(); wide != 1 || narrow <= wide {
		t.Fatalf("visual lines narrow=%d wide=%d", narrow, wide)
	}
	if transcript.Text() != "uno dos tres cuatro cinco" {
		t.Fatalf("logical text changed: %q", transcript.Text())
	}
}

// A saved tool result can be a single JSON line of hundreds of kilobytes. The
// info overlay renders it in full, so wrapping must stay linear in its size.
func TestTranscriptRendersLargeSingleLineQuickly(t *testing.T) {
	line := `{"tool":"plugins.call","result":"` + strings.Repeat("ceremony_definition ", 35_000) + `"}`
	start := time.Now()
	transcript := NewTranscript()
	transcript.SetWidth(120)
	transcript.Viewport.SetHeight(40)
	transcript.SetContent(line)
	transcript.Viewport.GotoTop()
	view := transcript.View()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("rendering a %d byte line took %s", len(line), elapsed)
	}
	if !strings.Contains(view, "plugins.call") {
		t.Fatalf("first line missing from view: %q", view)
	}
}
