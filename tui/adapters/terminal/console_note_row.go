package terminal

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

// consoleNotePrefix opens every message the console sends the model as a
// user message ("[AXLR]", "[AXLR · budget]", "[AXLR · memory]"…). The
// application counts the person's prompts with the same test.
const consoleNotePrefix = "[AXLR"

// consoleNote reports a user message the console wrote, not the person.
func consoleNote(content root.Text) bool {
	return strings.HasPrefix(string(content), consoleNotePrefix)
}

// splitConsoleNotes separates what the person typed from the console notes
// the console appended to it ("\n\n[AXLR] The person stopped the …"), or
// returns no typed text for a message the console wrote whole.
func splitConsoleNotes(content root.Text) (typed string, notes []string) {
	text := string(content)
	if !consoleNote(content) {
		at := strings.Index(text, "\n\n"+consoleNotePrefix)
		if at < 0 {
			return text, nil
		}
		typed, text = text[:at], text[at+2:]
	}
	for text != "" {
		next := strings.Index(text, "\n\n"+consoleNotePrefix)
		if next < 0 {
			notes = append(notes, text)
			break
		}
		notes = append(notes, text[:next])
		text = text[next+2:]
	}
	return typed, notes
}

// consoleNoteRow draws one console note as a dim single line that names
// what the console told the model; key identifies it so a click expands it
// to the full text. On 12 October 2026 a nine-line "[AXLR] The decompose
// step of ceremony axlr_plan is still open…" was drawn as a "›" prompt, in
// English, in the middle of a Spanish session.
func consoleNoteRow(note, key string, expanded bool, theme Theme) transcriptRow {
	row := transcriptRow{Label: theme.Icon("console") + " ", LabelTone: toneNone, Kind: transcriptRowConsole, Indent: true, Note: key}
	if consoleMemoryReminder(root.Text(note)) {
		row.Label, row.LabelTone, row.Kind = theme.Icon("memory")+" ", toneAccent, transcriptRowMemory
		row.Text = theme.T("transcript.memoryReminder")
	} else {
		row.Text = theme.Tf("console.row", consoleNoteSummary(note, theme))
	}
	if expanded {
		row.Text += "\n" + note
	}
	return row
}

var (
	noteStepOpen      = regexp.MustCompile(`(?i)\bthe (\S+) step of (?:ceremony )?\S+ is still open`)
	noteCeremonyStep  = regexp.MustCompile(`^\[AXLR\] Ceremony (\S+), current step: ([^\s.(]+)`)
	noteStopped       = regexp.MustCompile(`^\[AXLR\] The person stopped the (\S+) ceremony`)
	noteCancelled     = regexp.MustCompile(`^\[AXLR\] The console cancelled ceremony (\S+)`)
	noteJobMerged     = regexp.MustCompile(`^\[AXLR\] Self-(?:repair|improvement) (\S+) merged pull request #(\d+)`)
	noteJobBlocked    = regexp.MustCompile(`^\[AXLR\] Self-(?:repair|improvement) (\S+) ended BLOCKED`)
	noteJobFailed     = regexp.MustCompile(`^\[AXLR\] Self-(?:repair|improvement) (\S+) failed`)
	noteJobInterrupt  = regexp.MustCompile(`^\[AXLR\] Self-(?:repair|improvement) (\S+) was interrupted`)
	noteJevEstimate   = regexp.MustCompile(`estimates an? (\d+)% probability`)
	noteTag           = regexp.MustCompile(`^\[AXLR[^\]]*\]\s*`)
	noteFirstSentence = regexp.MustCompile(`^(.*?[.:;])(\s|$)`)
)

// consoleNoteSummary names, in the person's language, what a console note
// told the model; a note it does not recognise is summarised by its first
// sentence as written.
func consoleNoteSummary(note string, theme Theme) string {
	first, _, _ := strings.Cut(note, "\n")
	switch {
	case strings.HasPrefix(first, "[AXLR · budget]"):
		return theme.T("console.budget")
	case strings.HasPrefix(first, "[AXLR · Jev]"):
		// The line carries Jev's estimate, so the person sees how strong
		// the doubt was without expanding it (14 October 2026).
		if m := noteJevEstimate.FindStringSubmatch(first); m != nil {
			return theme.Tf("console.jevEstimate", m[1])
		}
		return theme.T("console.jev")
	case strings.HasPrefix(first, "[AXLR · recall]"):
		return theme.T("console.recall")
	case strings.HasPrefix(first, "[AXLR · restore]"):
		return theme.T("console.restore")
	case strings.HasPrefix(first, "[AXLR] The person approved the plan"):
		return theme.T("console.planApproved")
	case strings.HasPrefix(first, "[AXLR] The person declined the plan"):
		return theme.T("console.planDeclined")
	case strings.HasPrefix(first, "[AXLR] The person returned the plan"):
		return theme.T("console.planReturned")
	case strings.HasPrefix(first, "[AXLR] The person approved the postmortem"):
		return theme.T("console.postmortemApproved")
	case strings.HasPrefix(first, "[AXLR] The person sent the draft back"):
		return theme.T("console.draftReturned")
	case strings.HasPrefix(first, "[AXLR] The person decided"):
		return theme.T("console.decided")
	}
	if m := noteStopped.FindStringSubmatch(first); m != nil {
		return theme.Tf("console.stopped", m[1])
	}
	if m := noteCancelled.FindStringSubmatch(first); m != nil {
		return theme.Tf("console.cancelled", m[1])
	}
	if m := noteStepOpen.FindStringSubmatch(first); m != nil {
		return theme.Tf("console.stepOpen", m[1])
	}
	if m := noteCeremonyStep.FindStringSubmatch(first); m != nil {
		return theme.Tf("console.ceremonyStep", m[1], m[2])
	}
	if m := noteJobMerged.FindStringSubmatch(first); m != nil {
		return theme.Tf("console.jobMerged", m[1], m[2])
	}
	for _, job := range []struct {
		pattern *regexp.Regexp
		label   string
	}{{noteJobBlocked, "console.jobBlocked"}, {noteJobFailed, "console.jobFailed"}, {noteJobInterrupt, "console.jobInterrupted"}} {
		if m := job.pattern.FindStringSubmatch(first); m != nil {
			return theme.Tf(job.label, m[1])
		}
	}
	body := noteTag.ReplaceAllString(first, "")
	if m := noteFirstSentence.FindStringSubmatch(body); m != nil {
		body = strings.TrimRight(m[1], ".:;")
	}
	return ansi.Truncate(singleLine(body), 80, "…")
}
