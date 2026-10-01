package terminal

import (
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// editorialRows re-lays a Margen transcript for the Editorial theme: a
// speaker label opens every turn, prompts lose their marker, and a finished
// run of tool calls becomes one summary line. Runs with a call still waiting
// or running stay one line per call so their live state remains visible.
func editorialRows(rows []transcriptRow, theme Theme) []transcriptRow {
	var out []transcriptRow
	speaker := ""
	add := func(row transcriptRow, gap bool) {
		row.GapBefore = gap && len(out) > 0
		out = append(out, row)
	}
	open := func(name string, tone rowTone) {
		if speaker != name {
			add(transcriptRow{Label: name, LabelTone: tone, Kind: transcriptRowSpeaker}, true)
			speaker = name
		}
	}
	for i := 0; i < len(rows); i++ {
		row := rows[i]
		afterSpeaker := len(out) > 0 && out[len(out)-1].Kind == transcriptRowSpeaker
		switch {
		case row.Kind == transcriptRowUser:
			open(theme.T("editorial.you"), toneNone)
			row.Label, row.Indent = "", false
			// Consecutive prompts stay separate under one label.
			add(row, out[len(out)-1].Kind != transcriptRowSpeaker)
		case row.Kind.isTool() && row.Tool != nil:
			open("AXLR", toneAccent)
			afterSpeaker = out[len(out)-1].Kind == transcriptRowSpeaker
			end := i
			for end < len(rows) && rows[end].Kind.isTool() && rows[end].Tool != nil {
				end++
			}
			run := rows[i:end]
			if finishedRun(run) {
				add(toolSummaryRow(run, theme), !afterSpeaker)
			} else {
				for j, r := range run {
					add(r, j == 0 && !afterSpeaker)
				}
			}
			i = end - 1
		default:
			if row.Kind == transcriptRowAssistant {
				open("AXLR", toneAccent)
				afterSpeaker = out[len(out)-1].Kind == transcriptRowSpeaker
			}
			add(row, !afterSpeaker)
		}
	}
	return out
}

func finishedRun(run []transcriptRow) bool {
	for _, r := range run {
		if r.Tool.State == toolAwaiting || r.Tool.State == toolRunning {
			return false
		}
	}
	return true
}

// toolSummaryRow reads "used made_list_ceremony_definitions ×3 · axlr_skill ·
// 57.3 KB · 1.4 s", with failures and denials counted at the end.
func toolSummaryRow(run []transcriptRow, theme Theme) transcriptRow {
	var order []string
	counts := map[string]int{}
	bytes, failed, denied := 0, 0, 0
	var duration int64
	timed, memoryOnly := false, true
	for _, r := range run {
		f := r.Tool
		if counts[f.Label] == 0 {
			order = append(order, f.Label)
		}
		counts[f.Label]++
		bytes += f.Bytes
		if f.HasTime {
			duration += f.DurationMS
			timed = true
		}
		switch f.State {
		case toolFailed:
			failed++
		case toolDenied:
			denied++
		}
		memoryOnly = memoryOnly && r.Kind == transcriptRowMemory
	}
	names := make([]string, 0, len(order))
	for _, label := range order {
		if counts[label] > 1 {
			names = append(names, fmt.Sprintf("%s ×%d", label, counts[label]))
		} else {
			names = append(names, label)
		}
	}
	parts := []string{theme.Tf("editorial.used", strings.Join(names, " · "))}
	if bytes > 0 {
		parts = append(parts, formatBytes(bytes))
	}
	if timed {
		parts = append(parts, formatDuration(duration))
	}
	if failed > 0 {
		parts = append(parts, theme.Tf("editorial.failed", failed))
	}
	if denied > 0 {
		parts = append(parts, theme.Tf("editorial.denied", denied))
	}
	glyph, tone, kind := theme.Icon("done"), toneGood, transcriptRowPlain
	if memoryOnly {
		glyph, tone, kind = theme.Icon("memory"), toneAccent, transcriptRowMemory
	}
	if failed+denied > 0 {
		glyph, tone = theme.Icon("failed"), toneError
	}
	return transcriptRow{Label: glyph + " ", LabelTone: tone, Text: strings.Join(parts, " · "), Kind: kind, Indent: true, Tool: &toolFacts{State: toolDone}}
}

func (t Theme) editorial() bool { return t.ID == domain.ThemeEditorial }
