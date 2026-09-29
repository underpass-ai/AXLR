package terminal

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Transcript struct{ Viewport viewport.Model }

func NewTranscript() Transcript {
	v := viewport.New()
	v.SoftWrap = true
	return Transcript{Viewport: v}
}
func (t *Transcript) SetContent(s string) {
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetContent(Sanitize(s))
	if bottom {
		t.Viewport.GotoBottom()
	}
}
func (t *Transcript) SetSession(s domain.SessionState, draft string) {
	var b strings.Builder
	for _, m := range s.Messages {
		b.WriteString(string(m.Role) + ": " + string(m.Content) + "\n")
		for _, c := range m.ToolCalls {
			b.WriteString("tool request: " + string(c.Name) + " " + string(c.Arguments.Bytes()) + "\n")
			for _, a := range s.Activity {
				if a.Call.ID == c.ID {
					b.WriteString("decision: " + string(a.Decision) + "\n")
				}
			}
		}
	}
	if s.Draft != "" {
		b.WriteString("interrupted draft: " + string(s.Draft) + "\n")
	}
	if draft != "" {
		b.WriteString("assistant: " + draft)
	}
	t.SetContent(b.String())
}
func (t Transcript) View() string { return t.Viewport.View() }
