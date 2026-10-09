package terminal

import (
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

type transcriptRowKind uint8

const (
	transcriptRowPlain transcriptRowKind = iota
	transcriptRowUser
	transcriptRowAssistant
	transcriptRowMemory
	transcriptRowGap
	// transcriptRowSpeaker names who speaks next in the Editorial layout.
	transcriptRowSpeaker
	// transcriptRowDiffRemoved and transcriptRowDiffAdded are the lines an
	// approval card shows a file edit taking out and putting in.
	transcriptRowDiffRemoved
	transcriptRowDiffAdded
)

// transcriptRow is one conversation entry. The viewport wraps it to the
// available width, so its visual height follows its content.
type transcriptRow struct {
	Text      string
	Kind      transcriptRowKind
	GapBefore bool
	// Label precedes Text verbatim, coloured by LabelTone; Markdown renders
	// Text as model markdown. Indent hangs continuation lines under the text
	// rather than under the label.
	Label     string
	LabelTone rowTone
	Markdown  bool
	Indent    bool
	// Tool describes a tool row so the Editorial layout can summarise runs.
	Tool *toolFacts
	// At is when the row's message was added (zero when unknown); Aside is
	// shown right-aligned on the row's first line when it fits.
	At    time.Time
	Aside string
}

// toolFacts is what a tool row shows, kept structured for summaries.
type toolFacts struct {
	Label      string
	State      toolState
	Bytes      int
	DurationMS int64
	HasTime    bool
}

type toolState uint8

const (
	toolDone toolState = iota
	toolFailed
	toolDenied
	toolAwaiting
	toolRunning
)

// rowTone names a palette role for a row's leading glyph.
type rowTone uint8

const (
	toneNone rowTone = iota
	toneAccent
	toneGood
	toneWarning
	toneError
)

func (k transcriptRowKind) isTool() bool {
	return k == transcriptRowPlain || k == transcriptRowMemory
}

// consoleMemoryReminder reports the user message the console sends to ask
// the model to record memory. On 10 October 2026 it was drawn as a prompt,
// "› [AXLR · memory] KMP is connected…", as if the person had typed it.
func consoleMemoryReminder(content root.Text) bool {
	return strings.HasPrefix(string(content), application.MemoryReminderPrefix)
}
