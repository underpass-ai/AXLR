package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestInlineApprovalKeepsTheConversationInView(t *testing.T) {
	for _, size := range [][2]int{{50, 15}, {100, 30}} {
		m := update(approvalModel(t), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v := m.View().Content
		plain := ansi.Strip(v)
		if lipgloss.Width(v) > size[0] || lipgloss.Height(v) != size[1] {
			t.Fatalf("%v: view is %dx%d", size, lipgloss.Width(v), lipgloss.Height(v))
		}
		for _, want := range []string{"AXLR", "waiting for approval", "Confirm tool", "exact-target.txt", "a approve"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%v: inline approval lacks %q:\n%s", size, want, plain)
			}
		}
		if strings.Contains(plain, "enter send") || strings.Contains(plain, "ctrl+p actions") {
			t.Fatalf("%v: the footer offers keys that do nothing while deciding:\n%s", size, plain)
		}
		if strings.Index(plain, "waiting for approval") > strings.Index(plain, "Confirm tool") {
			t.Fatalf("%v: the card is not below the conversation:\n%s", size, plain)
		}
		if m.View().Cursor != nil {
			t.Fatalf("%v: the editor cursor shows while a decision is pending", size)
		}
		m.zones.Close()
	}
}
