package terminal

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// naturalOn is a console with /natural-writing on, typing without waiting
// and with a fixed random source.
func naturalOn(t *testing.T) AppModel {
	t.Helper()
	pace := naturalRhythm
	naturalRhythm = naturalPace{}
	t.Cleanup(func() { naturalRhythm = pace })
	m := sized()
	t.Cleanup(m.zones.Close)
	m.Composer.Input.SetValue("/natural-writing")
	m = update(m, ControlIntent("send"))
	if !m.UIPreferences.NaturalWriting || m.Composer.Input.Value() != "" {
		t.Fatalf("/natural-writing did not turn the mode on: on=%v draft=%q", m.UIPreferences.NaturalWriting, m.Composer.Input.Value())
	}
	m.natural.random = rand.New(rand.NewPCG(1, 2))
	return m
}

// tick delivers the typing's next tick, as its timer would.
func tick(t *testing.T, m AppModel, cmd tea.Cmd) (AppModel, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("the typing stopped at %q", m.Composer.Input.Value())
	}
	msg, ok := cmd().(naturalTick)
	if !ok {
		t.Fatal("the typing's command is not its tick")
	}
	next, cmd := m.Update(msg)
	return next.(AppModel), cmd
}

// A paste is typed a character at a time, newlines and all, and waits
// for the person's Enter.
func TestNaturalWritingTypesAPasteInCharacterByCharacter(t *testing.T) {
	m := naturalOn(t)
	text := "Take it for a spin.\r\nTotal, plus five names."
	want := strings.ReplaceAll(text, "\r\n", "\n")
	next, cmd := m.Update(tea.PasteMsg{Content: text})
	m = next.(AppModel)
	if got := m.Composer.Input.Value(); got != "T" {
		t.Fatalf("after the paste the composer holds %q; want only its first character", got)
	}
	typed := 1
	for cmd != nil {
		m, cmd = tick(t, m, cmd)
		typed++
		if got := m.Composer.Input.Value(); got != want[:typed] {
			t.Fatalf("after %d ticks the composer holds %q; want %q", typed, got, want[:typed])
		}
	}
	if typed != len([]rune(want)) || m.Composer.Input.Value() != want || m.typing() {
		t.Fatalf("typed %d characters, composer %q", typed, m.Composer.Input.Value())
	}
	if m.Busy || len(m.Header.State.Messages) != 0 {
		t.Fatal("the typed paste was sent on its own")
	}
	m.Status.Notice = ""
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "· natural writing") {
		t.Fatalf("the footer does not say the mode is on:\n%s", view)
	}
}

// Any key, Enter and Esc included, finishes the typing at once and does
// nothing else; a tick still on its way then changes nothing.
func TestAKeyFinishesTheTypingWithoutSendingOrCancelling(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: tea.KeyEscape}, {Code: 'x', Text: "x"}} {
		m := naturalOn(t)
		m = typeDraft(m, "Prompt: ")
		text := "list the stores Lambdas\nand their errors"
		next, cmd := m.Update(tea.PasteMsg{Content: text})
		m = next.(AppModel)
		m, cmd = tick(t, m, cmd)
		m, late := tick(t, m, cmd)
		if !m.typing() {
			t.Fatal("the paste was typed whole before the key")
		}
		next, cmd = m.Update(key)
		m = next.(AppModel)
		if got := m.Composer.Input.Value(); got != "Prompt: "+text || m.typing() || cmd != nil {
			t.Fatalf("%s: composer %q, typing %v", key.String(), got, m.typing())
		}
		if m.Busy || len(m.Header.State.Messages) != 0 {
			t.Fatalf("%s sent the prompt", key.String())
		}
		next, cmd = m.Update(late())
		m = next.(AppModel)
		if m.Composer.Input.Value() != "Prompt: "+text || cmd != nil {
			t.Fatalf("a late tick changed the composer to %q", m.Composer.Input.Value())
		}
		// Enter now sends, as it always does.
		m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if !m.Busy {
			t.Fatalf("after %s, Enter did not send", key.String())
		}
		m.cancel()
	}
}

// A click during a typing finishes it before it does anything else.
func TestAClickFinishesTheTyping(t *testing.T) {
	m := naturalOn(t)
	next, cmd := m.Update(tea.PasteMsg{Content: "five names, please"})
	m = next.(AppModel)
	m, _ = tick(t, m, cmd)
	m = update(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1})
	if m.typing() || m.Composer.Input.Value() != "five names, please" {
		t.Fatalf("after a click the composer holds %q, typing %v", m.Composer.Input.Value(), m.typing())
	}
}

// A second paste during a typing finishes the first and types itself after it.
func TestASecondPasteFollowsTheFirst(t *testing.T) {
	m := naturalOn(t)
	next, _ := m.Update(tea.PasteMsg{Content: "one two"})
	m = next.(AppModel)
	next, cmd := m.Update(tea.PasteMsg{Content: " three"})
	m = next.(AppModel)
	for cmd != nil {
		m, cmd = tick(t, m, cmd)
	}
	if got := m.Composer.Input.Value(); got != "one two three" {
		t.Fatalf("composer = %q", got)
	}
}

// With the mode on, a long paste is still its token at once; with it off,
// a paste appears at once as before.
func TestLongPastesAndTheModeOffPasteAtOnce(t *testing.T) {
	m := naturalOn(t)
	long := numberedLines(200)
	next, cmd := m.Update(tea.PasteMsg{Content: long})
	m = next.(AppModel)
	if cmd != nil || m.typing() || m.Composer.Input.Value() != pasteToken(m.Theme, long) {
		t.Fatalf("a long paste was typed: %q", m.Composer.Input.Value())
	}
	m.Composer.Input.Reset()
	m.pastes = nil
	m.Composer.Input.SetValue("/escritura-natural")
	m = update(m, ControlIntent("send"))
	if m.UIPreferences.NaturalWriting {
		t.Fatal("/escritura-natural did not turn the mode off")
	}
	if view := ansi.Strip(m.View().Content); strings.Contains(view, "natural writing on") || strings.Contains(view, "· natural writing") {
		t.Fatalf("the footer still shows the mode:\n%s", view)
	}
	next, cmd = m.Update(tea.PasteMsg{Content: "short\nprompt"})
	m = next.(AppModel)
	if cmd != nil || m.typing() || m.Composer.Input.Value() != "short\nprompt" {
		t.Fatalf("with the mode off the paste reads %q", m.Composer.Input.Value())
	}
}

// The pace: a base with jitter, longer after a space, a punctuation mark
// and a line, the same for the same random source.
func TestNaturalPaceVariesAndPausesAfterPunctuation(t *testing.T) {
	pace := naturalPace{base: 30 * time.Millisecond, jitter: 40 * time.Millisecond, space: 60 * time.Millisecond, punctuation: 200 * time.Millisecond, line: 400 * time.Millisecond}
	waits := func(seed uint64) []time.Duration {
		random := rand.New(rand.NewPCG(seed, 7))
		var out []time.Duration
		for _, r := range "ab c.\nd" {
			out = append(out, pace.after(r, random))
		}
		return out
	}
	first, again := waits(3), waits(3)
	for i := range first {
		if first[i] != again[i] {
			t.Fatalf("the same seed gave %v and %v", first, again)
		}
	}
	within := func(d, lo, hi time.Duration) bool { return d >= lo && d <= hi }
	letter, letter2, space, dot, line := first[0], first[1], first[2], first[4], first[5]
	if !within(letter, 30*time.Millisecond, 70*time.Millisecond) || !within(letter2, 30*time.Millisecond, 70*time.Millisecond) {
		t.Fatalf("letters wait %v and %v", letter, letter2)
	}
	if !within(space, 60*time.Millisecond, 130*time.Millisecond) {
		t.Fatalf("a space waits %v", space)
	}
	if !within(dot, 130*time.Millisecond, 270*time.Millisecond) {
		t.Fatalf("a full stop waits %v", dot)
	}
	if !within(line, 230*time.Millisecond, 470*time.Millisecond) {
		t.Fatalf("a new line waits %v", line)
	}
	varied := false
	random := rand.New(rand.NewPCG(5, 5))
	w := pace.after('a', random)
	for range 20 {
		if pace.after('a', random) != w {
			varied = true
		}
	}
	if !varied {
		t.Fatal("the pace has no jitter")
	}
}

// The mode is a UI preference: /natural-writing saves it, a console opened
// with it saved starts with it on, and a failed save changes nothing.
func TestNaturalWritingIsSavedAsAUIPreference(t *testing.T) {
	store := &uiPreferenceStub{}
	m := sized()
	defer m.zones.Close()
	m.deps.UIPreferenceStore = store
	m.Composer.Input.SetValue("/natural_writing")
	m = update(m, ControlIntent("send"))
	if !store.saved.NaturalWriting || store.saved.Theme != m.UIPreferences.Theme || !m.UIPreferences.NaturalWriting {
		t.Fatalf("saved %+v", store.saved)
	}
	// /theme saves the preferences whole and keeps the mode.
	m = update(m, ControlIntent("theme"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if store.saved.Theme != domain.ThemeInk || !store.saved.NaturalWriting {
		t.Fatalf("/theme saved %+v", store.saved)
	}

	opened := New(Dependencies{UIPreferences: domain.UIPreferences{Theme: domain.ThemeAuto, Icons: domain.IconsSafe, NaturalWriting: true}, Monochrome: true})
	defer opened.zones.Close()
	opened = update(opened, tea.WindowSizeMsg{Width: 100, Height: 30})
	if _, cmd := opened.Update(tea.PasteMsg{Content: "typed"}); cmd == nil {
		t.Fatal("a console opened with the preference saved pastes at once")
	}

	store.err = errors.New("disk full")
	m.Composer.Input.SetValue("/natural-writing")
	m = update(m, ControlIntent("send"))
	if !m.UIPreferences.NaturalWriting || !strings.Contains(m.Status.Error, "disk full") {
		t.Fatalf("a failed save turned the mode off: on=%v error=%q", m.UIPreferences.NaturalWriting, m.Status.Error)
	}
}
