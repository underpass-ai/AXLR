package terminal

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/aymanbagabas/go-udiff"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
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
		Why     string  `json:"why"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Path == nil {
		return nil, false
	}
	// The model's reason for the change comes first, before the diff.
	var reason []transcriptRow
	if why := strings.TrimSpace(call.Why); why != "" {
		reason = append(reason, transcriptRow{Text: theme.Tf("approval.why", strings.ReplaceAll(why, "\n", " ")), Kind: transcriptRowSpeaker})
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
		rows := append(append([]transcriptRow(nil), reason...), header)
		if context, ok := editRows(*call.OldText, *call.NewText); ok {
			return append(rows, context...), true
		}
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
			body = append(body[:writePreviewLines], transcriptRow{Text: theme.Tn("approval.moreLines", hidden, hidden), Kind: transcriptRowPlain})
		}
		return append(append(append([]transcriptRow(nil), reason...), header), body...), true
	}
	return nil, false
}

// editRows lays an edit out as a line diff of its old and new text: lines
// both share are context, drawn once and unmarked. On 10 October 2026 an
// edit of "// comment\nconst X" to "// other\nconst X" drew the unchanged
// const line twice, as removed and as added. ok is false when the texts are
// equal or cannot be diffed; the card then lists them whole.
func editRows(oldText, newText string) ([]transcriptRow, bool) {
	before, after := strings.TrimSuffix(oldText, "\n")+"\n", strings.TrimSuffix(newText, "\n")+"\n"
	if newText == "" {
		after = ""
	}
	if before == after {
		return nil, false
	}
	diff, err := udiff.ToUnifiedDiff("", "", before, udiff.Lines(before, after), strings.Count(before, "\n")+strings.Count(after, "\n"))
	if err != nil || len(diff.Hunks) != 1 {
		return nil, false
	}
	var rows []transcriptRow
	for _, line := range diff.Hunks[0].Lines {
		row := transcriptRow{Label: "  ", Text: strings.ReplaceAll(strings.TrimSuffix(line.Content, "\n"), "\t", "    "), Kind: transcriptRowAssistant, Indent: true}
		switch line.Kind {
		case udiff.Delete:
			row.Label, row.LabelTone, row.Kind = "- ", toneError, transcriptRowDiffRemoved
		case udiff.Insert:
			row.Label, row.LabelTone, row.Kind = "+ ", toneGood, transcriptRowDiffAdded
		}
		rows = append(rows, row)
	}
	return rows, true
}

// setRows shows rows as they are, one after another with no gap between
// them, as a card's details do.
func (t *Transcript) setRows(rows []transcriptRow) {
	t.rows = rows
	t.renderRows()
}

// writeDiffRows lays a local_write over an existing file out as its diff
// against the file on disk, like an edit's card: the reason, the path and
// mode, how many lines it adds and removes, then removed lines red, added
// lines green and unchanged ones once as context. On 14 October 2026 a
// replace card listed the whole new calc.go and the person had to find the
// two lines it changed. ok is false for other calls and when the diff
// cannot be drawn; the card then shows the content.
func writeDiffRows(arguments []byte, onDisk string, theme Theme) ([]transcriptRow, bool) {
	var call struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
		Mode    string  `json:"mode"`
		Why     string  `json:"why"`
	}
	if json.Unmarshal(arguments, &call) != nil || call.Content == nil {
		return nil, false
	}
	change := domain.FileChange{Path: root.RelativePath(call.Path), Before: root.Text(onDisk), After: root.Text(*call.Content)}
	diff, ok := changeResultRows(change, theme)
	if !ok || len(diff) < 2 {
		return nil, false
	}
	preview := previewChange(change)
	var rows []transcriptRow
	if why := strings.TrimSpace(call.Why); why != "" {
		rows = append(rows, transcriptRow{Text: theme.Tf("approval.why", strings.ReplaceAll(why, "\n", " ")), Kind: transcriptRowSpeaker})
	}
	header := "── " + call.Path + " ──"
	if call.Mode != "" {
		header = "── " + call.Path + " · " + call.Mode + " ──"
	}
	rows = append(rows, transcriptRow{Text: header, Kind: transcriptRowSpeaker}, transcriptRow{Text: theme.Tf("approval.writeDiff", preview.Added, preview.Removed), Kind: transcriptRowConsole})
	body := diff[1:] // changeResultRows names the path first
	if hidden := len(body) - writePreviewLines; hidden > 0 {
		body = append(body[:writePreviewLines], transcriptRow{Text: theme.Tn("approval.moreLines", hidden, hidden), Kind: transcriptRowPlain})
	}
	return append(rows, body...), true
}

// writtenFile is what a pending local_write's file holds on disk, read
// inside the workspace (a path that leaves it is not read), when it exists
// and fits a change preview.
func (m AppModel) writtenFile(p domain.PendingTool) (string, bool) {
	if p.Call.Name != "local_write" {
		return "", false
	}
	var call struct {
		Path string `json:"path"`
	}
	workspace := string(m.deps.Workspace)
	if workspace == "" {
		workspace = string(m.Header.State.Workspace)
	}
	if json.Unmarshal(p.Call.Arguments.Bytes(), &call) != nil || workspace == "" || !filepath.IsLocal(filepath.FromSlash(call.Path)) {
		return "", false
	}
	dir, err := os.OpenRoot(workspace)
	if err != nil {
		return "", false
	}
	defer dir.Close()
	file, err := dir.Open(filepath.FromSlash(call.Path))
	if err != nil {
		return "", false
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > domain.MaxChangePreviewBytes {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(file, domain.MaxChangePreviewBytes+1))
	if err != nil || !domain.ChangePreviewFits(string(data)) || !utf8.Valid(data) {
		return "", false
	}
	return string(data), true
}
