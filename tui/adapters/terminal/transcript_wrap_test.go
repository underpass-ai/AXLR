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
// info overlay renders it in full, so the work must stay linear in its size.
// Until 481511b8 the viewport soft-wrapped and re-sliced the whole line from
// its start on every render, and a 657 KB result stalled the console at 100%
// CPU. Transcript now wraps once, in SetContent or when SetWidth changes the
// width, and a render only touches the rows on screen.
//
// Until 24 October 2026 this test gave one render a 2 s budget. On 23 October
// 2026 macos-15-intel took 2.78 s under -race in a PR that did not touch the
// console (this machine takes 0.4 s) and the native tests had to be rerun. A
// runner's speed cannot change the mechanism, so the test checks that: the
// line is wrapped exactly once, renders and a same-width SetWidth wrap
// nothing, a new width wraps once more, the viewport keeps its own soft wrap
// off and every wrapped line fits, so a render has nothing left to wrap. The
// clock is still read, but against one ansi.Wrap of the same bytes on the
// same machine at the same moment: a render costs a small multiple of that
// (3–8× measured here, with and without -race), where work growing with the
// square of the size would be thousands of times, and three attempts outlast
// a scheduler stall.
func TestTranscriptRendersLargeSingleLineQuickly(t *testing.T) {
	line := `{"tool":"plugins.call","result":"` + strings.Repeat("ceremony_definition ", 35_000) + `"}`
	wraps, wrappedBytes := 0, 0
	wrapText = func(s string, limit int, breakpoints string) string {
		wraps++
		wrappedBytes += len(s)
		return ansi.Wrap(s, limit, breakpoints)
	}
	t.Cleanup(func() { wrapText = ansi.Wrap })

	transcript := NewTranscript()
	transcript.SetWidth(120)
	transcript.Viewport.SetHeight(40)
	transcript.SetContent(line)
	if wraps != 1 || wrappedBytes != len(line) {
		t.Fatalf("SetContent wrapped %d times over %d bytes; want once over the %d byte line", wraps, wrappedBytes, len(line))
	}
	transcript.Viewport.GotoTop()
	view := transcript.View()
	if !strings.Contains(view, "plugins.call") {
		t.Fatalf("first line missing from view: %q", view)
	}
	if rows := strings.Count(view, "\n") + 1; rows > 40 {
		t.Fatalf("a render of a 40 row viewport produced %d rows", rows)
	}
	for range 3 {
		transcript.View()
	}
	transcript.SetWidth(120)
	if wraps != 1 {
		t.Fatalf("three renders and a same-width SetWidth wrapped %d more times", wraps-1)
	}
	if transcript.Viewport.SoftWrap {
		t.Fatal("the viewport soft-wraps again, so it re-slices the whole line on every render")
	}
	for i, wrapped := range strings.Split(transcript.Viewport.GetContent(), "\n") {
		if w := ansi.StringWidth(wrapped); w > 120 {
			t.Fatalf("wrapped line %d is %d cells wide, so the viewport has wrapping left to do", i, w)
		}
	}
	transcript.SetWidth(100)
	if wraps != 2 {
		t.Fatalf("a new width wrapped %d times; want once", wraps-1)
	}

	// The render may cost renderOverWrap times one wrap of its text. The
	// baseline is floored because Go's clocks on Windows advance in steps of
	// up to 15.6 ms, which could read a short wrap as nothing.
	const renderOverWrap, attempts = 50, 3
	for attempt := 1; ; attempt++ {
		started := time.Now()
		ansi.Wrap(line, 120, "")
		baseline := max(time.Since(started), 50*time.Millisecond)
		started = time.Now()
		fresh := NewTranscript()
		fresh.SetWidth(120)
		fresh.Viewport.SetHeight(40)
		fresh.SetContent(line)
		fresh.Viewport.GotoTop()
		fresh.View()
		render := time.Since(started)
		if render <= renderOverWrap*baseline {
			break
		}
		if attempt == attempts {
			t.Fatalf("rendering the %d byte line took %s, over %d× the %s one wrap of it takes here, in all %d attempts", len(line), render, renderOverWrap, baseline, attempts)
		}
	}
}
