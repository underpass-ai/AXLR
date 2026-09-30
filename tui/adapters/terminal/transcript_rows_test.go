package terminal

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestTranscriptAssistantRowsFillWidthAndGrowWithText(t *testing.T) {
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(30)
	transcript.Viewport.SetHeight(10)
	state := domain.SessionState{Messages: []root.Message{
		{Role: root.RoleUser, Content: "question"},
		{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("long answer ", 20))},
	}}
	transcript.SetSession(state, "", Theme{})
	if transcript.Viewport.TotalLineCount() <= 3 {
		t.Fatalf("answer did not grow with wrapped text: %d lines", transcript.Viewport.TotalLineCount())
	}
	if got := lipgloss.Height(transcript.View()); got != 10 {
		t.Fatalf("wrapped viewport height = %d; want 10", got)
	}
	if transcript.Viewport.StyleLineFunc != nil {
		t.Fatal("styling before viewport wrap risks adding visual rows")
	}
	if !strings.Contains(transcript.View(), "48;2;27;32;48") {
		t.Fatal("assistant rows have no background")
	}
}

func TestTranscriptAssistantRowsRespectMonochrome(t *testing.T) {
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(50)
	transcript.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: "answer"}}}, "", Theme{Monochrome: true})
	if strings.Contains(transcript.View(), "48;2;") {
		t.Fatal("monochrome transcript has a background")
	}
}

func TestTranscriptRowsFollowThemeAndResize(t *testing.T) {
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(50)
	transcript.Viewport.SetHeight(3)
	transcript.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: "answer"}}}, "", Theme{})
	dark := transcript.View()
	transcript.Viewport.SetWidth(70)
	transcript.ApplyTheme(Theme{Light: true})
	light := transcript.View()
	if strings.Split(light, "\n")[0] == strings.Split(dark, "\n")[0] || lipgloss.Width(light) != 70 {
		t.Fatalf("assistant row did not follow terminal theme and width: dark=%q light=%q width=%d", dark, light, lipgloss.Width(light))
	}
}

func TestTranscriptRowsStayInsideViewportAfterScrollAndResize(t *testing.T) {
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(30)
	transcript.Viewport.SetHeight(5)
	transcript.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("long answer ", 20))}}}, "", Theme{})
	for _, width := range []int{30, 23, 50} {
		transcript.Viewport.SetWidth(width)
		transcript.ApplyTheme(Theme{})
		transcript.Viewport.GotoBottom()
		if got := lipgloss.Height(transcript.View()); got != 5 {
			t.Fatalf("width %d: height = %d; want 5", width, got)
		}
		if got := lipgloss.Width(transcript.View()); got != width {
			t.Fatalf("width %d: rendered width = %d", width, got)
		}
		transcript.Viewport.GotoTop()
		if got := lipgloss.Height(transcript.View()); got != 5 {
			t.Fatalf("width %d at top: height = %d", width, got)
		}
	}
}

func TestTranscriptUnsentPromptKeepsAssistantRowColor(t *testing.T) {
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(40)
	transcript.Viewport.SetHeight(4)
	transcript.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleAssistant, Content: "saved answer"}}}, "", Theme{})
	transcript.AppendUnsent([]string{"retry me"})
	lines := strings.Split(transcript.View(), "\n")
	if !strings.Contains(lines[0], "48;2;27;32;48") || strings.Contains(lines[1], "48;2;") || !strings.Contains(lines[2], "48;2;32;60;72") || !strings.Contains(strings.Join(lines, "\n"), "Not sent: retry me") {
		t.Fatalf("assistant color or unsent prompt lost: %q", transcript.View())
	}
}

func TestTranscriptInterleavesRowsAndScrollsWhenTheyExceedTerminal(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("first question", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("first partial answer"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("second question", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "second answer"}}); err != nil {
		t.Fatal(err)
	}
	transcript := NewTranscript()
	transcript.Viewport.SetWidth(30)
	transcript.Viewport.SetHeight(3)
	transcript.SetSession(s.Export(), "", Theme{})
	content := transcript.Viewport.GetContent()
	for _, item := range []string{"user: first question", "interrupted draft: first partial answer", "user: second question", "assistant: second answer"} {
		if !strings.Contains(content, item) {
			t.Fatalf("missing row %q in %q", item, content)
		}
	}
	if strings.Index(content, "user: first") > strings.Index(content, "interrupted draft") || strings.Index(content, "interrupted draft") > strings.Index(content, "user: second") || strings.Index(content, "user: second") > strings.Index(content, "assistant: second") {
		t.Fatalf("rows not in conversation order: %q", content)
	}
	if transcript.Viewport.TotalLineCount() <= 3 || lipgloss.Height(transcript.View()) != 3 {
		t.Fatal("rows do not scroll inside a fixed height")
	}
	transcript.Viewport.GotoTop()
	if !strings.Contains(transcript.View(), "first question") || strings.Contains(transcript.View(), "second answer") {
		t.Fatal("top of scrollable conversation is wrong")
	}
	transcript.Viewport.GotoBottom()
	if !strings.Contains(transcript.View(), "second answer") || lipgloss.Width(transcript.View()) != 30 {
		t.Fatal("bottom row is missing or not full width")
	}
}
