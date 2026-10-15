package terminal

import (
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// naturalWriting is /natural-writing, for recording demos: while it is on,
// a paste into the composer is typed in a character at a time at a human
// pace instead of appearing at once. Any key finishes the typing and puts
// the rest in; nothing is ever sent without the person's Enter. Whether
// it is on is a UI preference, saved in settings.json.
type naturalWriting struct {
	// pending is what is left to type; gen tells the ticks of the typing
	// under way from those of a typing finished or replaced.
	pending []rune
	gen     uint64
	random  *rand.Rand
}

// naturalTick types the next character of the typing gen.
type naturalTick struct{ gen uint64 }

// naturalPace is how long the typing waits after a character: a base with
// some jitter, longer after a space, a punctuation mark or a line.
type naturalPace struct {
	base, jitter, space, punctuation, line time.Duration
}

// naturalRhythm is the console's pace, about 15 characters a second; tests
// replace it to type without waiting.
var naturalRhythm = naturalPace{base: 35 * time.Millisecond, jitter: 45 * time.Millisecond, space: 50 * time.Millisecond, punctuation: 180 * time.Millisecond, line: 350 * time.Millisecond}

// after is the wait once r is typed: the extra pause varies too, from
// half to all of it.
func (p naturalPace) after(r rune, random *rand.Rand) time.Duration {
	vary := func(d time.Duration) time.Duration {
		if d <= 0 {
			return 0
		}
		return d/2 + time.Duration(random.Int64N(int64(d/2)+1))
	}
	wait := p.base + time.Duration(random.Int64N(int64(p.jitter)+1))
	switch {
	case r == '\n':
		wait += vary(p.line)
	case r == ' ' || r == '\t':
		wait += vary(p.space)
	case strings.ContainsRune(".,;:!?…", r):
		wait += vary(p.punctuation)
	}
	return wait
}

// toggleNaturalWriting turns the mode on or off and saves it; a typing
// under way is finished first.
func (m AppModel) toggleNaturalWriting() (AppModel, tea.Cmd) {
	m = m.finishTyping()
	next := m.UIPreferences
	next.NaturalWriting = !next.NaturalWriting
	if store := m.deps.UIPreferenceStore; store != nil {
		if err := store.Save(m.lifetime.ctx, next); err != nil {
			m.Status.Error = m.Theme.T("error.saveNaturalWriting") + err.Error()
			return m, nil
		}
	}
	m.UIPreferences = next
	m.Composer.Input.Reset()
	m.Status.Error = ""
	m.Status.Notice = m.Theme.T("notice.naturalWritingOff")
	if next.NaturalWriting {
		m.Status.Notice = m.Theme.T("notice.naturalWritingOn")
	}
	return m, nil
}

// typePaste starts typing a paste into the composer. A long paste still
// reads as its token at once, so ok is false and paste handles it.
func (m AppModel) typePaste(text string) (next AppModel, cmd tea.Cmd, ok bool) {
	m = m.finishTyping()
	// tmux and GNOME Terminal send a pasted newline as a carriage return.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if text == "" || lineCount(text) > pasteFoldLines {
		return m, nil, false
	}
	if m.natural.random == nil {
		m.natural.random = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	}
	m.natural.gen++
	m.natural.pending = []rune(text)
	m, cmd = m.typeNext(m.natural.gen)
	return m, cmd, true
}

// typeNext types one character and waits for the next.
func (m AppModel) typeNext(gen uint64) (AppModel, tea.Cmd) {
	if gen != m.natural.gen || len(m.natural.pending) == 0 {
		return m, nil
	}
	r := m.natural.pending[0]
	m.natural.pending = m.natural.pending[1:]
	m.Composer.Input.InsertString(string(r))
	m.slashSelected = 0
	m.resizeComposer()
	if len(m.natural.pending) == 0 {
		return m, nil
	}
	return m, tea.Tick(naturalRhythm.after(r, m.natural.random), func(time.Time) tea.Msg { return naturalTick{gen: gen} })
}

// finishTyping puts in at once what a typing under way has left; its
// next tick then finds nothing to do.
func (m AppModel) finishTyping() AppModel {
	if len(m.natural.pending) == 0 {
		return m
	}
	m.Composer.Input.InsertString(string(m.natural.pending))
	m.natural.pending = nil
	m.natural.gen++
	m.slashSelected = 0
	m.resizeComposer()
	return m
}

// typing reports whether a paste is being typed.
func (m AppModel) typing() bool {
	return len(m.natural.pending) > 0
}
