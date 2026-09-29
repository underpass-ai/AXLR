package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestLayoutSizes(t *testing.T) {
	for _, tc := range []struct {
		w, h       int
		side, tiny bool
	}{{100, 30, true, false}, {70, 20, false, false}, {40, 10, false, true}} {
		m := update(New(Dependencies{Monochrome: true}), tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
		if m.Layout.SidePanel != tc.side || m.Layout.TooSmall != tc.tiny {
			t.Fatalf("layout: %+v", m.Layout)
		}
		got := m.View().Content
		if tc.tiny && !strings.Contains(got, "Resize") {
			t.Fatal(got)
		}
		if lipgloss.Width(got) > tc.w || lipgloss.Height(got) > tc.h {
			t.Fatalf("overflow %dx%d: %dx%d", tc.w, tc.h, lipgloss.Width(got), lipgloss.Height(got))
		}
	}
}
func TestLayoutWideGlyphWrapping(t *testing.T) {
	m := sized()
	m.Transcript.SetContent(strings.Repeat("界🙂", 100))
	got := m.View().Content
	if lipgloss.Width(got) > 100 || lipgloss.Height(got) > 30 {
		t.Fatal("wide glyph overflow")
	}
}

func TestLayoutUntrustedMultilineMetadataDoesNotExpandChrome(t *testing.T) {
	m := sized()
	m.Header.State.Workspace = "/tmp/\n\n\n\n\n\n\n\n\n\nname"
	m.Status.Error = strings.Repeat("error\n", 20)
	got := m.View().Content
	if lipgloss.Height(got) > 30 {
		t.Fatal("metadata expanded beyond terminal")
	}
}
