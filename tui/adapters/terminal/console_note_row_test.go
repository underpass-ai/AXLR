package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const stepOpenNote = "[AXLR] The decompose step of ceremony axlr_plan is still open. If its work is done, hand it back now with axlr_step_done.\nThe plan must list the tasks.\nThird line of instructions."

// consoleNoteSession is a planning session in which the console reminded the
// model of an open step, noted its budget, relayed a decision and appended
// the stop note to the person's next prompt.
func consoleNoteSession() domain.SessionState {
	return domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Messages: []root.Message{
		{Role: root.RoleUser, Content: "planifica IsDev()"},
		{Role: root.RoleAssistant, Content: "Este es el plan."},
		{Role: root.RoleUser, Content: stepOpenNote},
		{Role: root.RoleAssistant, Content: "Hecho."},
		{Role: root.RoleUser, Content: "[AXLR · budget] 2 calls left of this turn's 32. Finish the task with what you have."},
		{Role: root.RoleUser, Content: "[AXLR] The person approved the plan. Tell the user in their language; the console runs its tasks."},
		{Role: root.RoleUser, Content: "y ahora los tests\n\n[AXLR] The person stopped the axlr_plan ceremony; do not resume it or call axlr_step_done unless asked."},
		{Role: root.RoleAssistant, Content: "Vale."},
	}}
}

// On 12 October 2026 "[AXLR] The decompose step of ceremony axlr_plan is
// still open…" was drawn as a nine-line "›" prompt, in English, in a
// Spanish session; budget notes and ceremony decisions too.
func TestConsoleNotesAreOneDimLineNotPrompts(t *testing.T) {
	for locale, want := range map[Locale][]string{
		English: {"⟲ console · reminded the model to hand back the decompose step", "⟲ console · reminded the model of its tool-call limit", "⟲ console · told the model the plan was approved", "⟲ console · told the model you stopped the axlr_plan ceremony"},
		Spanish: {"⟲ consola · recordó al modelo que entregue el paso decompose", "⟲ consola · recordó al modelo su límite de llamadas", "⟲ consola · avisó al modelo de que el plan está aprobado", "⟲ consola · avisó al modelo de que paraste la ceremonia axlr_plan"},
	} {
		tr := NewTranscript()
		tr.SetWidth(120)
		tr.SetSession(consoleNoteSession(), "", Theme{Locale: locale})
		text := ansi.Strip(tr.Text())
		for _, line := range want {
			if !strings.Contains(text, line+"\n") && !strings.HasSuffix(text, line) {
				t.Fatalf("%s: no console line %q:\n%s", locale, line, text)
			}
		}
		if strings.Contains(text, "[AXLR") || strings.Contains(text, "still open") || strings.Contains(text, "Third line") {
			t.Fatalf("%s: a console note is drawn in full:\n%s", locale, text)
		}
		for _, prompt := range []string{"› planifica IsDev()", "› y ahora los tests"} {
			if !strings.Contains(text, prompt) {
				t.Fatalf("%s: the typed prompt %q lost its bubble:\n%s", locale, prompt, text)
			}
		}
		if n := strings.Count(text, "›"); n != 2 {
			t.Fatalf("%s: %d prompt bubbles, want 2:\n%s", locale, n, text)
		}
	}
	// A note it does not recognise reads as its first sentence.
	if got := consoleNoteSummary("[AXLR] Something new happened. Then more.", Theme{}); got != "Something new happened" {
		t.Fatalf("fallback summary = %q", got)
	}
	// Jev's line carries its estimate (14 October 2026).
	jev := "[AXLR · Jev] An independent check that saw only the person's request, your last answer and the tool activity estimates a 27% probability that your answer completes the request."
	if got := consoleNoteSummary(jev, Theme{Locale: Spanish}); got != "trasladó al modelo la duda de Jev sobre la respuesta final (27 %)" {
		t.Fatalf("Jev summary = %q", got)
	}
	if got := consoleNoteSummary(jev, Theme{}); got != "passed on Jev's doubt about the final answer (27%)" {
		t.Fatalf("Jev summary = %q", got)
	}
	if got := consoleNoteSummary("[AXLR · restore] Since your last answer the person restored calc/calc.go to what it held before your call w1.", Theme{Locale: Spanish}); got != "avisó al modelo de los archivos que restauraste" {
		t.Fatalf("restore summary = %q", got)
	}
	if got := consoleNoteSummary("[AXLR] Nobody is reading this repair session: the build step of axlr_repair is still open (reminder 1 of 3).", Theme{}); got != "reminded the model to hand back the build step" {
		t.Fatalf("repair nudge summary = %q", got)
	}
}

func TestPromptHistoryLeavesConsoleNotesOut(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Header.State = consoleNoteSession()
	m.resetPromptHistory()
	if strings.Join(m.promptHistory, "|") != "planifica IsDev()|y ahora los tests" {
		t.Fatalf("history = %q", m.promptHistory)
	}
}

// A click on a console note opens its full text; another click folds it.
func TestClickingAConsoleNoteExpandsAndFoldsIt(t *testing.T) {
	now := time.Now()
	selectionNow = func() time.Time { now = now.Add(time.Second); return now }
	defer func() { selectionNow = time.Now }()
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &session, Monochrome: true}), tea.WindowSizeMsg{Width: 120, Height: 40})
	defer m.zones.Close()
	m.Header.State = consoleNoteSession()
	m.refreshTranscript()
	clickNote := func(m AppModel, summary string) AppModel {
		t.Helper()
		lines := strings.Split(ansi.Strip(m.Transcript.Viewport.GetContent()), "\n")
		for i, line := range lines {
			if strings.Contains(line, summary) {
				if _, ok := m.Transcript.NoteAt(i); !ok {
					t.Fatalf("line %d %q is not a note", i, line)
				}
				y := 1 + i - m.Transcript.Viewport.YOffset()
				m = update(m, tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
				return update(m, tea.MouseReleaseMsg{X: 6, Y: y, Button: tea.MouseLeft})
			}
		}
		t.Fatalf("no line shows %q:\n%s", summary, strings.Join(lines, "\n"))
		return m
	}
	m = clickNote(m, "hand back the decompose step")
	text := m.Transcript.Text()
	if !strings.Contains(text, "If its work is done") || !strings.Contains(text, "Third line of instructions.") || m.Transcript.Selection.Set {
		t.Fatalf("the click did not expand the note:\n%s", text)
	}
	if strings.Contains(text, "2 calls left") {
		t.Fatal("the click expanded another note")
	}
	// The note stays open as the transcript is rebuilt.
	m.refreshTranscript()
	if !strings.Contains(m.Transcript.Text(), "Third line of instructions.") {
		t.Fatal("the expanded note folded on refresh")
	}
	m = clickNote(m, "hand back the decompose step")
	if strings.Contains(m.Transcript.Text(), "Third line of instructions.") {
		t.Fatalf("the second click did not fold the note:\n%s", m.Transcript.Text())
	}
	// A click on a prompt expands nothing.
	m = clickNote(m, "you stopped the axlr_plan ceremony")
	m = clickNote(m, "you stopped the axlr_plan ceremony")
	before := m.Transcript.Text()
	lines := strings.Split(ansi.Strip(m.Transcript.Viewport.GetContent()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "planifica IsDev()") {
			y := 1 + i - m.Transcript.Viewport.YOffset()
			m = update(m, tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
			m = update(m, tea.MouseReleaseMsg{X: 6, Y: y, Button: tea.MouseLeft})
		}
	}
	if m.Transcript.Text() != before {
		t.Fatal("a click on a prompt changed the transcript")
	}
}

func TestAFoldedConsoleNoteStaysOneLineWhenNarrow(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(40)
	tr.SetSession(consoleNoteSession(), "", Theme{Monochrome: true})
	for i := range tr.VisualLineCount() {
		if key, ok := tr.NoteAt(i); ok && key == "2.0" {
			if next, ok := tr.NoteAt(i + 1); ok && next == key {
				t.Fatalf("the folded note wraps:\n%s", tr.Viewport.GetContent())
			}
			return
		}
	}
	t.Fatalf("no note row:\n%s", tr.Viewport.GetContent())
}
