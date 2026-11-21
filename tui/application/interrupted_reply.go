package application

import (
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// interruptedReplyTail is how much of an interrupted reply the model reads
// back: its end, where it stopped, within a small window's means.
const interruptedReplyTail = 1500

// markInterrupted tells the model, on the prompt that followed it, what its
// reply showed when it was cut short: by the person (Esc) or by a failure,
// such as the model's server going away mid-reply. The draft stays on screen as
// an interrupted reply but was in no message, so asked where it had
// stopped, a local model answered from an earlier reply (wrx80, 21 November
// 2026). The stored prompt is unchanged; only the request reads the note.
func markInterrupted(messages []root.Message, drafts []domain.ArchivedDraft) []root.Message {
	marked := messages
	copied := false
	for _, draft := range drafts {
		i := draft.AfterMessage
		if i < 0 || i >= len(messages) || messages[i].Role != root.RoleUser || draft.Content == "" {
			continue
		}
		if !copied {
			marked = append([]root.Message(nil), messages...)
			copied = true
		}
		marked[i].Content += root.Text("\n\n[AXLR · Your previous reply was cut short before it finished (stopped by the person or by a failure). It had shown: «" + replyTail(string(draft.Content)) + "»]")
	}
	return marked
}

// replyTail is the end of text, at most interruptedReplyTail bytes, cut at
// a rune boundary and marked when cut.
func replyTail(text string) string {
	if len(text) <= interruptedReplyTail {
		return text
	}
	start := len(text) - interruptedReplyTail
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return "…" + text[start:]
}
