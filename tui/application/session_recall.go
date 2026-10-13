package application

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The recall note (decisions D3 to D6 of the lifecycle integration
// design): at a session's first prompt the console wakes the workspace's
// memory and the person's, and appends what it found to the prompt as a
// console note, so the model starts from what the project settled without
// registering, waking or paging KMP itself. A resumed session gets its own
// history woken too; a later prompt on another topic gets a fresh note.

// RecallNotePrefix opens the note; the transcript shows it as a console
// row and the memory reminder leaves it alone (it is not "[AXLR · memory]").
const RecallNotePrefix = "[AXLR · recall]"

// Recall modes, as axlr_recall names them.
const (
	RecallWake    = "wake"
	RecallAsk     = "ask"
	RecallHistory = "history"
)

// RecallRequest is one read of memory under an about: a wake (with an
// intent the store may focus on), a question, or a span of history (Since
// onwards, else the newest Entries).
type RecallRequest struct {
	Mode, About, Intent, Since string
	Entries                    int
	ContextID                  string
}

// RecallResult is what the read gave: prose and refs for a wake, a compact
// answer for a question or a history. Empty says nothing is recorded under
// the about.
type RecallResult struct {
	Mode, About string
	Text        string
	Refs        []string
	Answer      map[string]any
	Empty       bool
}

// RecallPort reads KMP for the console and for axlr_recall.
type RecallPort interface {
	Recall(ctx context.Context, request RecallRequest) (RecallResult, error)
}

// recalledIntentBytes bounds what the console keeps of the woken prompt.
const recalledIntentBytes = 600

// Recall wakes memory for the prompt the session is about to begin and
// returns the note to append to it, or "" when the prompt continues the
// topic last woken. It opens the session's KMP context first, so the wake
// runs in it. The note is bounded as a ceremony's recall is (2 KiB; 1 KiB
// under the compact profile), shared by the abouts in order: the project's,
// the person's, and the session's own when the session is resumed.
func (c *SessionCeremony) Recall(ctx context.Context, s *domain.Session, prompt string) string {
	if !c.drives(*s) || c.Identity == nil || c.Identity.Recall == nil {
		return ""
	}
	if s.Mode().StartsCeremony() || s.Mode() == domain.ModePlan {
		return "" // the ceremony wakes its own about; the planner reads code
	}
	c.openContext(ctx, s)
	instance, _ := s.Instance()
	id := s.ID()
	resumed := len(s.Messages()) > 0 && !c.seen(id)
	if instance.Recalled != "" && !resumed && !topicChanged(instance.Recalled, prompt) {
		c.markSeen(id)
		return ""
	}
	c.markSeen(id)
	limit := maxCeremonyMemoryBytes
	if _, _, compact := compactRun(*s); compact {
		limit = compactMemoryBytes
	}
	state := s.Export()
	abouts := []string{c.about(ctx, state)}
	if c.Identity.UserAbout != "" {
		abouts = append(abouts, c.Identity.UserAbout)
	}
	if resumed {
		abouts = append(abouts, sessionAbout(id))
	}
	if instance.Predecessor != "" && instance.Recalled == "" {
		// A successor's first recall wakes what the predecessor settled.
		abouts = append(abouts, sessionAbout(domain.SessionID(instance.Predecessor)))
	}
	var sections []string
	remaining := limit
	instance.RecalledMemories = nil
	for i, about := range abouts {
		if remaining <= 0 {
			break
		}
		section, lines := c.recallSection(ctx, about, prompt, instance.KMPContext, i == 0, remaining)
		remaining -= len(section) + 1
		sections = append(sections, section)
		if i == 0 {
			instance.RecalledMemories = lines
		}
	}
	instance.Recalled = bounded(prompt, recalledIntentBytes)
	if err := s.SetInstance(instance); err != nil {
		c.log("session instance: %v", err)
	}
	c.log("memory recalled for session %s: %s (%d bytes)", id, strings.Join(abouts, ", "), limit-remaining)
	return RecallNotePrefix + " " + strings.Join(sections, "\n")
}

// recallSection wakes one about and renders it within limit bytes. Nothing
// recorded is said in one line (decision D6: "new project, nothing
// recorded", nothing more); a failure is disclosed, never hidden. The
// memories exposed come back too, "ref text" per line, for Jev.
func (c *SessionCeremony) recallSection(ctx context.Context, about, prompt, contextID string, project bool, limit int) (string, []string) {
	result, err := c.Identity.Recall.Recall(ctx, RecallRequest{Mode: RecallWake, About: about, Intent: prompt, ContextID: contextID})
	if err != nil {
		c.log("recall of %s: %v", about, err)
		return bounded(fmt.Sprintf("%s: KMP recall unavailable: %s", about, bounded(err.Error(), 200)), limit), nil
	}
	if result.Empty || strings.TrimSpace(result.Text) == "" {
		if project {
			return bounded(about+": new project, nothing recorded yet.", limit), nil
		}
		return bounded(about+": nothing recorded yet.", limit), nil
	}
	header := "Memory of " + about + ":\n"
	return header + bounded(result.Text, max(0, limit-len(header))), recalledMemories(result.Text, result.Refs)
}

// recalledMemoryLines bounds what the sidecar keeps of a recall for Jev.
const (
	recalledMemoryLines = 8
	recalledMemoryBytes = 300
)

// recalledMemories pairs the refs a wake exposed with their lines: a wake
// line reads "- <ref> (kind): text", and only lines that start with a
// known ref are kept.
func recalledMemories(text string, refs []string) []string {
	known := map[string]bool{}
	for _, ref := range refs {
		known[ref] = true
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		ref, rest, _ := strings.Cut(line, " ")
		if !known[ref] {
			continue
		}
		out = append(out, bounded(ref+" "+strings.TrimSpace(rest), recalledMemoryBytes))
		if len(out) == recalledMemoryLines {
			break
		}
	}
	return out
}

// seen and markSeen track the sessions this console incarnation already
// recalled: a session with messages that this process never recalled was
// resumed, and its own history is woken once.
func (c *SessionCeremony) seen(id domain.SessionID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recalled[id]
}

func (c *SessionCeremony) markSeen(id domain.SessionID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.recalled == nil {
		c.recalled = map[domain.SessionID]bool{}
	}
	c.recalled[id] = true
}

// topicChanged reports whether a prompt leaves the topic last woken: the
// words of four letters or more it shares with the woken prompt are fewer
// than a fifth of its own. A short prompt (under four such words: "sí",
// "sigue", "y los tests") continues the topic, whatever its words.
func topicChanged(last, prompt string) bool {
	current := topicWords(prompt)
	if len(current) < 4 {
		return false
	}
	previous := topicWords(last)
	shared := 0
	for word := range current {
		if previous[word] {
			shared++
		}
	}
	return shared*5 < len(current)
}

func topicWords(text string) map[string]bool {
	words := map[string]bool{}
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(field)) >= 4 {
			words[field] = true
		}
	}
	return words
}
