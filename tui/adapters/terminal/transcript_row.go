package terminal

type transcriptRowKind uint8

const (
	transcriptRowPlain transcriptRowKind = iota
	transcriptRowUser
	transcriptRowAssistant
	transcriptRowMemory
	transcriptRowGap
	// transcriptRowSpeaker names who speaks next in the Editorial layout.
	transcriptRowSpeaker
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
