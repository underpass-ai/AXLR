package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFooterHintsStateAndTokensFitOneRow(t *testing.T) {
	for _, width := range []int{50, 70, 120} {
		m := update(sized(), tea.WindowSizeMsg{Width: width, Height: 20})
		m.tokens = 12_400
		footer := ansi.Strip(m.footerView())
		if strings.Contains(footer, "\n") || ansi.StringWidth(footer) != width {
			t.Fatalf("width %d: footer is not one full row: %q", width, footer)
		}
		if !strings.HasPrefix(footer, " enter send") || !strings.HasSuffix(strings.TrimRight(footer, " "), "idle · 12.4k tok") {
			t.Fatalf("width %d: footer %q", width, footer)
		}
		m.zones.Close()
	}
}

func TestFooterGivesErrorsTheWholeRow(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Status.Error = "session workspace /home/a/kmp differs from active workspace /home/a/AXLR"
	footer := ansi.Strip(m.footerView())
	if strings.Contains(footer, "enter send") || !strings.Contains(footer, "differs from active workspace /home/a/AXLR") {
		t.Fatalf("error not shown in full: %q", footer)
	}
}

func TestFormatTokens(t *testing.T) {
	for n, want := range map[int]string{980: "980", 12_400: "12.4k", 1_250_000: "1.2M"} {
		if got := formatTokens(n); got != want {
			t.Fatalf("formatTokens(%d) = %q; want %q", n, got, want)
		}
	}
}
