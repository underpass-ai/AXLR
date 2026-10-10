package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// On 10 October 2026 a pasted five-line prompt left the transcript at the
// previous answer: the composer grew, the viewport shrank under it, and the
// view no longer counted as following the bottom when the reply arrived.
func TestAMultiLinePromptKeepsTheTranscriptFollowingTheBottom(t *testing.T) {
	m := sized()
	for i := range 60 {
		m = update(m, application.Event{Kind: application.EventTextDelta, Text: root.Text("earlier answer line " + strings.Repeat("x", i%7) + "\n")})
	}
	m.Transcript.Viewport.GotoBottom()
	if !m.Transcript.Viewport.AtBottom() {
		t.Fatal("setup: transcript is not at the bottom")
	}
	m = update(m, tea.PasteMsg{Content: "one\ntwo\nthree\nfour\nfive"})
	if m.Composer.Input.Height() < 5 {
		t.Fatalf("setup: composer height %d did not grow with the draft", m.Composer.Input.Height())
	}
	if !m.Transcript.Viewport.AtBottom() {
		t.Fatal("growing the composer stopped following the bottom")
	}
	m = update(m, ControlIntent("send"))
	defer m.cancel()
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	for range 20 {
		m = update(m, application.Event{Kind: application.EventTextDelta, Text: "new reply line\n"})
	}
	if !m.Transcript.Viewport.AtBottom() {
		t.Fatalf("after sending, the transcript stopped at offset %d of %d lines", m.Transcript.Viewport.YOffset(), m.Transcript.VisualLineCount())
	}
}
