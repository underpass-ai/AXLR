package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestInlineApprovalKeepsTheConversationInView(t *testing.T) {
	for _, size := range [][2]int{{50, 15}, {100, 30}} {
		m := update(approvalModel(t), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v := m.View().Content
		plain := ansi.Strip(v)
		if lipgloss.Width(v) > size[0] || lipgloss.Height(v) != size[1] {
			t.Fatalf("%v: view is %dx%d", size, lipgloss.Width(v), lipgloss.Height(v))
		}
		// At 50 columns the row may wrap inside "waiting for approval".
		for _, want := range []string{"AXLR", "waiting", "Confirm tool", "exact-target.txt", "a approve"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%v: inline approval lacks %q:\n%s", size, want, plain)
			}
		}
		if strings.Contains(plain, "enter send") || strings.Contains(plain, "ctrl+p actions") {
			t.Fatalf("%v: the footer offers keys that do nothing while deciding:\n%s", size, plain)
		}
		if strings.Index(plain, "waiting") > strings.Index(plain, "Confirm tool") {
			t.Fatalf("%v: the card is not below the conversation:\n%s", size, plain)
		}
		if m.View().Cursor != nil {
			t.Fatalf("%v: the editor cursor shows while a decision is pending", size)
		}
		m.zones.Close()
	}
}

// On 14 October 2026, at 70 columns, the card's keys were cut at
// "l permitir siem…" and d (deny) and esc were not shown at all.
func TestTheApprovalCardWrapsItsKeysOnNarrowTerminals(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"go","args":["test","./..."]}`)
	defer m.zones.Close()
	m.Theme.Locale = Spanish
	m = update(m, tea.WindowSizeMsg{Width: 70, Height: 18})
	card := ansi.Strip(m.approvalCard())
	for _, want := range []string{"a aprobar", "l permitir siempre", "d denegar", "esc cancelar"} {
		if !strings.Contains(card, want) {
			t.Fatalf("the card lost %q:\n%s", want, card)
		}
	}
	for _, line := range strings.Split(card, "\n") {
		if ansi.StringWidth(line) > 70 {
			t.Fatalf("a row is wider than the screen: %q", line)
		}
	}
	// The view keeps its height: the conversation gives up the rows.
	if lines := strings.Split(m.View().Content, "\n"); len(lines) != 18 {
		t.Fatalf("the view is %d rows", len(lines))
	}
	if rows := wrapActions([]string{"a", "bb", "c"}, []int{1, 2, 1}, " ", "  ", 6); len(rows) != 2 || rows[0] != " a  bb" || rows[1] != " c" {
		t.Fatalf("%q", rows)
	}
}
