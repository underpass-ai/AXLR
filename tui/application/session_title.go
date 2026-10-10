package application

import (
	"context"
	"strings"
	"unicode"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// titlePartRunes bounds each prompt's part of a derived title, so two parts
// and their separator stay under domain.MaxSessionTitleRunes.
const titlePartRunes = 56

// titleUntitled gives an untitled session a title once its second request
// has completed. The axlr-session skill asks the model for one after the
// second prompt; on 10 October 2026 claude-haiku-5.5 loaded the skill and
// set none over seven prompts, and Ctrl+O listed two sessions by the same
// first prompt. The title is derived from the person's first two prompts:
// no model request, and the system prompt does not carry the title, so the
// prompt cache is untouched. A title set by the person or the model is never
// replaced (Initialize fills missing fields only). Best effort: a label store
// failure does not fail the turn.
func titleUntitled(ctx context.Context, s domain.Session, labels SessionLabelsPort) {
	if labels == nil || s.Status() != domain.StatusComplete {
		return
	}
	prompts := personPrompts(s.Messages(), 2)
	if len(prompts) < 2 {
		return
	}
	current, err := labels.Load(ctx)
	if err != nil || current[s.Export().ID].Title != "" {
		return
	}
	title := derivedTitle(prompts)
	if title == "" {
		return
	}
	_, _ = labels.Initialize(ctx, s.Export().ID, domain.SessionLabel{Title: root.Text(title)})
}

// personPrompts returns up to limit of the person's prompts, oldest first.
// Console messages, which all start with "[AXLR", are not theirs.
func personPrompts(messages []root.Message, limit int) []string {
	var prompts []string
	for _, m := range messages {
		if len(prompts) == limit {
			break
		}
		if m.Role == root.RoleUser && !strings.HasPrefix(string(m.Content), "[AXLR") {
			prompts = append(prompts, string(m.Content))
		}
	}
	return prompts
}

// derivedTitle joins the first line of each prompt, whitespace collapsed and
// cut at a word to titlePartRunes, with " · "; a repeated part is kept once.
func derivedTitle(prompts []string) string {
	var parts []string
	for _, prompt := range prompts {
		part := titlePart(prompt)
		if part != "" && (len(parts) == 0 || parts[len(parts)-1] != part) {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " · ")
}

func titlePart(prompt string) string {
	var line string
	for _, candidate := range strings.Split(prompt, "\n") {
		if line = strings.Join(strings.FieldsFunc(candidate, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " "); line != "" {
			break
		}
	}
	runes := []rune(line)
	if len(runes) <= titlePartRunes {
		return line
	}
	cut := titlePartRunes - 1
	for i := cut; i > titlePartRunes/2; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}
