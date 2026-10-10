package terminal

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// changeRecord is one completed write or edit, in the order it happened.
type changeRecord struct {
	ID     root.ToolCallID
	Change domain.FileChange
	Tool   string
	Turn   int
	At     time.Time
}

// changeFile groups a session's changes to one path. Its net change runs
// from the first step's before to the last step's after, like git diff.
type changeFile struct {
	Path    root.RelativePath
	Steps   []int // indices into records, oldest first
	Net     domain.FileChange
	preview changePreview
}

// changeRow is one line of the list: a file, or one of its steps when the
// file is expanded.
type changeRow struct {
	File int
	Step int // index into the file's Steps; -1 for the file itself
}

// ChangeViewer reviews a session's file changes per file, newest first. A
// file with several writes expands into its steps. Session snapshots remain
// its only data source; it never reads the live files.
type ChangeViewer struct {
	Theme         Theme
	Details       viewport.Model
	records       []changeRecord
	previews      []changePreview
	files         []changeFile
	expanded      map[root.RelativePath]bool
	rows          []changeRow
	selected      int
	window        int
	detailFocus   bool
	width, height int
	sessionID     domain.SessionID
}

func NewChangeViewer() ChangeViewer {
	return ChangeViewer{Details: viewport.New(), expanded: map[root.RelativePath]bool{}}
}

func (v *ChangeViewer) SetSession(state domain.SessionState) {
	records := changeRecords(state)
	if state.ID == v.sessionID && slices.EqualFunc(records, v.records, func(a, b changeRecord) bool { return a.ID == b.ID && a.Change == b.Change }) {
		return
	}
	var keep changeRow
	keepPath, keepStep := root.RelativePath(""), root.ToolCallID("")
	if state.ID == v.sessionID && v.selected < len(v.rows) {
		keep = v.rows[v.selected]
		keepPath = v.files[keep.File].Path
		if keep.Step >= 0 {
			keepStep = v.records[v.files[keep.File].Steps[keep.Step]].ID
		}
	} else {
		v.detailFocus = false
		v.expanded = map[root.RelativePath]bool{}
		v.Details.GotoTop()
	}
	previews := make([]changePreview, len(records))
	for i, record := range records {
		if found := slices.IndexFunc(v.records, func(r changeRecord) bool { return r.ID == record.ID && r.Change == record.Change }); found >= 0 {
			previews[i] = v.previews[found]
		} else {
			previews[i] = previewChange(record.Change)
		}
	}
	v.records, v.previews, v.sessionID = records, previews, state.ID
	v.files = groupChanges(records, previews)
	v.rebuildRows()
	v.selected, v.window = 0, 0
	for i, row := range v.rows {
		file := v.files[row.File]
		if file.Path == keepPath && (row.Step < 0 && keepStep == "" || row.Step >= 0 && v.records[file.Steps[row.Step]].ID == keepStep) {
			v.selected = i
		}
	}
	v.Resize(v.width, v.height)
}

// Open starts every review on the list, at the most recently changed file.
func (v *ChangeViewer) Open(state domain.SessionState) {
	v.SetSession(state)
	v.selected, v.window, v.detailFocus = 0, 0, false
	v.Details.GotoTop()
	v.Details.SetXOffset(0)
	v.renderDetail()
}

func changeRecords(state domain.SessionState) []changeRecord {
	resultAt := map[root.ToolCallID]int{}
	for i, m := range state.Messages {
		if m.Role == root.RoleTool {
			resultAt[m.ToolCallID] = i
		}
	}
	var records []changeRecord
	for _, a := range state.Activity {
		if a.Outcome == nil || a.Outcome.IsError || a.Outcome.Uncertain || a.Decision == domain.DecisionDeny || a.Outcome.Change == nil {
			continue
		}
		record := changeRecord{ID: a.Call.ID, Change: *a.Outcome.Change}
		record.Tool, _ = toolCallPresentation(state, a.Call)
		if index, ok := resultAt[a.Call.ID]; ok {
			for _, m := range state.Messages[:index] {
				if m.Role == root.RoleUser && !consoleMemoryReminder(m.Content) {
					record.Turn++
				}
			}
			if index < len(state.MessageTimes) {
				record.At = state.MessageTimes[index]
			}
		}
		records = append(records, record)
	}
	return records
}

func groupChanges(records []changeRecord, previews []changePreview) []changeFile {
	index := map[root.RelativePath]int{}
	var files []changeFile
	for i, record := range records {
		at, ok := index[record.Change.Path]
		if !ok {
			at = len(files)
			index[record.Change.Path] = at
			files = append(files, changeFile{Path: record.Change.Path})
		}
		files[at].Steps = append(files[at].Steps, i)
	}
	for i := range files {
		f := &files[i]
		first, last := records[f.Steps[0]].Change, records[f.Steps[len(f.Steps)-1]].Change
		if len(f.Steps) == 1 {
			f.Net, f.preview = first, previews[f.Steps[0]]
			continue
		}
		f.Net = domain.FileChange{Path: f.Path, Created: first.Created, Before: first.Before, After: last.After}
		for _, step := range f.Steps {
			if reason := records[step].Change.Unavailable; reason != "" {
				f.Net = domain.FileChange{Path: f.Path, Created: first.Created, Unavailable: reason}
				break
			}
		}
		f.preview = previewChange(f.Net)
	}
	// Most recently changed file first.
	slices.SortStableFunc(files, func(a, b changeFile) int { return b.Steps[len(b.Steps)-1] - a.Steps[len(a.Steps)-1] })
	return files
}

func (v *ChangeViewer) rebuildRows() {
	v.rows = v.rows[:0]
	for i, f := range v.files {
		v.rows = append(v.rows, changeRow{File: i, Step: -1})
		if v.expanded[f.Path] && len(f.Steps) > 1 {
			for k := len(f.Steps) - 1; k >= 0; k-- {
				v.rows = append(v.rows, changeRow{File: i, Step: k})
			}
		}
	}
}

func (v *ChangeViewer) Resize(width, height int) {
	v.width, v.height = width, height
	innerWidth, bodyHeight := OverlayBodySize(width, height)
	if width >= 90 {
		innerWidth -= v.listWidth() + 3
	}
	v.Details.SetWidth(max(1, innerWidth))
	v.Details.SetHeight(max(1, bodyHeight-3))
	v.renderDetail()
}

func (v ChangeViewer) listWidth() int { return min(40, max(26, v.width/3)) }

func (v *ChangeViewer) selectRow(index int) {
	if len(v.rows) == 0 {
		return
	}
	index = max(0, min(len(v.rows)-1, index))
	if index != v.selected {
		v.selected = index
		v.Details.GotoTop()
		v.Details.SetXOffset(0)
		v.renderDetail()
	}
}

// setExpanded opens or closes the selected file's steps, keeping the file
// row selected.
func (v *ChangeViewer) setExpanded(open bool) {
	if v.selected >= len(v.rows) {
		return
	}
	row := v.rows[v.selected]
	file := v.files[row.File]
	if len(file.Steps) < 2 || v.expanded[file.Path] == open {
		return
	}
	v.expanded[file.Path] = open
	v.rebuildRows()
	v.selected = slices.Index(v.rows, changeRow{File: row.File, Step: -1})
	v.renderDetail()
}

// stepFile moves to the previous or next file row.
func (v *ChangeViewer) stepFile(delta int) {
	if len(v.rows) == 0 {
		return
	}
	file := v.rows[v.selected].File + delta
	if file < 0 || file >= len(v.files) {
		return
	}
	v.selectRow(slices.Index(v.rows, changeRow{File: file, Step: -1}))
}

func (v *ChangeViewer) Update(msg tea.Msg, zones *zone.Manager, prefix string) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return "close"
		case "tab", "shift+tab":
			v.detailFocus = !v.detailFocus
			return ""
		case "[":
			v.stepFile(-1)
			return ""
		case "]":
			v.stepFile(1)
			return ""
		}
		if !v.detailFocus {
			switch k.String() {
			case "up", "down":
				delta := 1
				if k.String() == "up" {
					delta = -1
				}
				v.selectRow(v.selected + delta)
				return ""
			case "right":
				v.setExpanded(true)
				return ""
			case "left":
				if v.selected < len(v.rows) && v.rows[v.selected].Step >= 0 {
					v.selectRow(slices.Index(v.rows, changeRow{File: v.rows[v.selected].File, Step: -1}))
				}
				v.setExpanded(false)
				return ""
			case "enter":
				if v.selected < len(v.rows) {
					row := v.rows[v.selected]
					if row.Step < 0 && len(v.files[row.File].Steps) > 1 {
						v.setExpanded(!v.expanded[v.files[row.File].Path])
						return ""
					}
				}
				v.detailFocus = true
				return ""
			case "pgup", "pgdown", "home", "end":
				v.detailFocus = true
			}
		}
		if k.String() == "home" {
			v.Details.GotoTop()
			return ""
		}
		if k.String() == "end" {
			v.Details.GotoBottom()
			return ""
		}
	}
	if mouse, ok := msg.(tea.MouseClickMsg); ok && mouse.Button == tea.MouseLeft {
		if zones.Get(prefix + "close").InBounds(mouse) {
			return "close"
		}
		if zones.Get(prefix + "detail").InBounds(mouse) {
			v.detailFocus = true
		}
		for i := range v.rows {
			if zones.Get(fmt.Sprintf("%schange-%d", prefix, i)).InBounds(mouse) {
				if i == v.selected && v.rows[i].Step < 0 {
					v.setExpanded(!v.expanded[v.files[v.rows[i].File].Path])
				} else {
					v.selectRow(i)
				}
				v.detailFocus = v.width < 90
				return ""
			}
		}
	}
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		v.detailFocus = true
	}
	if v.detailFocus {
		v.Details, _ = v.Details.Update(msg)
	}
	return ""
}

// selection is the change and preview the detail shows: a file's net change
// or one of its steps.
func (v ChangeViewer) selection() (domain.FileChange, changePreview, *changeRecord) {
	row := v.rows[v.selected]
	file := v.files[row.File]
	if row.Step < 0 {
		return file.Net, file.preview, nil
	}
	index := file.Steps[row.Step]
	return v.records[index].Change, v.previews[index], &v.records[index]
}

func (v *ChangeViewer) renderDetail() {
	v.Details.Style = lipgloss.NewStyle()
	if !v.Theme.Monochrome {
		p := v.Theme.palette()
		v.Details.Style = v.Details.Style.Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Background))
	}
	if len(v.rows) == 0 {
		v.Details.SetContent("")
		return
	}
	c, preview, _ := v.selection()
	content := preview.render(v.Theme, c.Created && c.Before == "")
	if c.Unavailable != "" {
		content = v.Theme.Muted(ansi.Wrap(v.Theme.T("changes."+c.Unavailable), max(1, v.Details.Width()), ""))
	} else if content == "" {
		content = v.Theme.Muted(v.Theme.T("changes.emptyFile"))
	}
	offset := v.Details.YOffset()
	v.Details.SetContent(content)
	v.Details.SetYOffset(offset)
}

func (v *ChangeViewer) View(zones *zone.Manager, prefix string, width, height int) string {
	v.Resize(width, height)
	w, h := OverlayBodySize(width, height)
	footer := zones.Mark(prefix+"close", "["+v.Theme.T("common.close")+"]")
	if len(v.rows) == 0 {
		body := v.Theme.Heading(v.Theme.T("changes.empty")) + "\n\n" + v.Theme.Muted(v.Theme.T("changes.emptyHint"))
		return v.Theme.Overlay(v.Theme.T("changes.title"), v.Theme.T("changes.subtitle"), body, footer, width, height)
	}
	listWidth := w
	if width >= 90 {
		listWidth = v.listWidth()
	}
	pageSize := max(1, h-1)
	if v.selected < v.window {
		v.window = v.selected
	}
	if v.selected >= v.window+pageSize {
		v.window = v.selected - pageSize + 1
	}
	list := []string{v.Theme.Accent(ansi.Truncate(v.countLine(), listWidth, "…"))}
	for i := v.window; i < min(len(v.rows), v.window+pageSize); i++ {
		list = append(list, zones.Mark(fmt.Sprintf("%schange-%d", prefix, i), v.listRow(i, listWidth)))
	}
	for len(list) < h {
		list = append(list, "")
	}
	detail := v.detailHeader() + "\n" + v.Details.View()
	if !v.Theme.Monochrome {
		detail = lipgloss.NewStyle().Width(v.Details.Width()).Height(h).Background(lipgloss.Color(v.Theme.palette().Background)).Render(detail)
	}
	detail = zones.Mark(prefix+"detail", detail)
	body := strings.Join(list, "\n")
	if width >= 90 {
		divider := v.Theme.Muted(strings.TrimSuffix(strings.Repeat(" │ \n", h), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Height(h).Render(body), divider, detail)
	} else if v.detailFocus {
		body = detail
	}
	hint := v.Theme.T("changes.listHint")
	if v.detailFocus {
		hint = v.Theme.T("changes.detailHint")
	}
	footer += "  " + v.position()
	return v.Theme.Overlay(v.Theme.T("changes.title"), hint, body, footer, width, height)
}

// listRow draws a file ("▸ M src/main.go ×3   +4 -1") or one of its steps
// ("    turn 2 · 14:02 · local_edit   +2 -1").
func (v ChangeViewer) listRow(i, width int) string {
	row := v.rows[i]
	file := v.files[row.File]
	c, preview, record := v.rowChange(i)
	stats := fmt.Sprintf("+%d -%d", preview.Added, preview.Removed)
	if c.Unavailable != "" {
		stats = "…"
	}
	var label string
	if record == nil {
		kind := "M"
		if c.Created {
			kind = "A"
		}
		marker := "  "
		if len(file.Steps) > 1 {
			marker = "▸ "
			if v.expanded[file.Path] {
				marker = "▾ "
			}
		}
		label = marker + kind + " " + singleLine(string(file.Path))
		if len(file.Steps) > 1 {
			label += fmt.Sprintf(" ×%d", len(file.Steps))
		}
	} else {
		label = "    " + v.stepLabel(*record)
	}
	label = ansi.Truncate(label, max(1, width-ansi.StringWidth(stats)-3), "…")
	gap := strings.Repeat(" ", max(1, width-1-ansi.StringWidth(label)-ansi.StringWidth(stats)))
	if i == v.selected {
		line := label + gap + stats
		if v.Theme.Monochrome {
			return "> " + ansi.Truncate(line, max(1, width-2), "")
		}
		return v.Theme.Selected(" " + line)
	}
	return " " + label + gap + v.Theme.Muted(stats)
}

// rowChange is selection for any row; v is a copy, so the real selection
// does not move.
func (v ChangeViewer) rowChange(i int) (domain.FileChange, changePreview, *changeRecord) {
	v.selected = i
	return v.selection()
}

func (v ChangeViewer) stepLabel(r changeRecord) string {
	parts := []string{v.Theme.Tf("changes.turn", r.Turn)}
	if clock := formatClock(r.At, transcriptNow()); clock != "" {
		parts = append(parts, clock)
	}
	if r.Tool != "" {
		parts = append(parts, singleLine(r.Tool))
	}
	return strings.Join(parts, " · ")
}

// detailHeader names what the diff shows: the file and its net change, or
// one step with its turn, time and tool.
func (v ChangeViewer) detailHeader() string {
	c, preview, record := v.selection()
	width := v.Details.Width()
	file := v.files[v.rows[v.selected].File]
	path := v.Theme.reviewLine(ansi.Truncate(singleLine(string(file.Path)), width, "…"), v.Theme.palette().Accent)
	var what string
	switch {
	case record != nil:
		what = v.Theme.Tf("changes.stepOf", v.rows[v.selected].Step+1, len(file.Steps)) + " · " + v.stepLabel(*record)
	case len(file.Steps) > 1:
		what = v.Theme.Tf("changes.netOf", len(file.Steps))
	case len(file.Steps) == 1:
		what = v.stepLabel(v.records[file.Steps[0]])
	}
	if c.Unavailable == "" {
		what += "  " + v.Theme.changeColor(fmt.Sprintf("+%d", preview.Added), true) + " " + v.Theme.changeColor(fmt.Sprintf("-%d", preview.Removed), false)
	}
	return path + "\n" + v.Theme.reviewLine(ansi.Truncate(what, width, "…"), v.Theme.palette().Text) + "\n" + v.Theme.reviewLine(v.Theme.T("changes.columns"), v.Theme.palette().Muted)
}

// position is one numbering for list, header and footer: file i of n, and
// the step when one is selected.
func (v ChangeViewer) position() string {
	row := v.rows[v.selected]
	text := v.Theme.Tf("changes.filePosition", row.File+1, len(v.files))
	if row.Step >= 0 {
		text += " · " + v.Theme.Tf("changes.stepOf", row.Step+1, len(v.files[row.File].Steps))
	}
	return text
}

func (t Theme) changeColor(text string, added bool) string {
	if t.Monochrome {
		return text
	}
	color := t.palette().DiffRemoved
	if added {
		color = t.palette().Good
	}
	return t.reviewLine(text, color)
}

func (t Theme) reviewLine(text, color string) string {
	if t.Monochrome {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(lipgloss.Color(t.palette().Background)).Render(text)
}

// countLine reads "2 files · 3 changes", with singular forms.
func (v ChangeViewer) countLine() string {
	files, changes := v.Theme.Tf("changes.files", len(v.files)), v.Theme.Tf("changes.changes", len(v.records))
	if len(v.files) == 1 {
		files = v.Theme.T("changes.oneFile")
	}
	if len(v.records) == 1 {
		changes = v.Theme.T("changes.oneChange")
	}
	return files + " · " + changes
}
