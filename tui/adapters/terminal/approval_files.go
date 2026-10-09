package terminal

import (
	"encoding/json"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
)

// writePreviewLines caps how much of a written file the card lists; the
// rest is counted, and the change panel (ctrl+d) shows it all afterwards.
const writePreviewLines = 400

// fileChangeRows lays a local_edit or local_write call out as the change it
// makes rather than as JSON with escaped newlines and tabs (seen on 10 Oct
// 2026, a 15-line local_edit was one unreadable blob of \n and \t): the
// path, then an edit's old text as "-" lines and its new text as "+" lines,
// or the written content. ok is false for other tools and for arguments
// that do not have the tool's shape; the card then shows them as JSON.
func fileChangeRows(name root.ToolName, arguments []byte, theme Theme) ([]transcriptRow, bool) {
	var call struct {
		Path    *string `json:"path"`
		OldText *string `json:"old_text"`
		NewText *string `json:"new_text"`
		Content *string `json:"content"`
		Mode    string  `json:"mode"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Path == nil {
		return nil, false
	}
	lines := func(text, label string, tone rowTone, kind transcriptRowKind) []transcriptRow {
		var rows []transcriptRow
		for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			rows = append(rows, transcriptRow{Label: label, LabelTone: tone, Text: strings.ReplaceAll(line, "\t", "    "), Kind: kind, Indent: true})
		}
		return rows
	}
	header := transcriptRow{Text: "── " + *call.Path + " ──", Kind: transcriptRowSpeaker}
	switch name {
	case "local_edit":
		if call.OldText == nil || call.NewText == nil {
			return nil, false
		}
		rows := []transcriptRow{header}
		rows = append(rows, lines(*call.OldText, "- ", toneError, transcriptRowDiffRemoved)...)
		if *call.NewText != "" {
			rows = append(rows, lines(*call.NewText, "+ ", toneGood, transcriptRowDiffAdded)...)
		}
		return rows, true
	case "local_write":
		if call.Content == nil {
			return nil, false
		}
		if call.Mode != "" {
			header.Text = "── " + *call.Path + " · " + call.Mode + " ──"
		}
		body := lines(*call.Content, "", toneNone, transcriptRowAssistant)
		if hidden := len(body) - writePreviewLines; hidden > 0 {
			body = append(body[:writePreviewLines], transcriptRow{Text: theme.Tf("approval.moreLines", hidden), Kind: transcriptRowPlain})
		}
		return append([]transcriptRow{header}, body...), true
	}
	return nil, false
}

// setRows shows rows as they are, one after another with no gap between
// them, as a card's details do.
func (t *Transcript) setRows(rows []transcriptRow) {
	t.rows = rows
	t.renderRows()
}
