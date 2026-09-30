package terminal

type transcriptRowKind uint8

const (
	transcriptRowPlain transcriptRowKind = iota
	transcriptRowUser
	transcriptRowAssistant
	transcriptRowMemory
	transcriptRowGap
)

// transcriptRow is one conversation entry. The viewport wraps it to the
// available width, so its visual height follows its content.
type transcriptRow struct {
	Text      string
	Kind      transcriptRowKind
	GapBefore bool
}
