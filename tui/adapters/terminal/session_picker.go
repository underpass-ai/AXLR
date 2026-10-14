package terminal

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionPicker lists saved sessions newest first. By default it shows the
// active workspace's sessions that have messages, without the sessions a
// plan's tasks ran in; Tab widens it to every workspace, then lists the plan
// tasks' sessions, then the archived ones, and typing filters by title.
type SessionPicker struct {
	Items         []domain.SessionSummary
	Selected      int
	Workspace     domain.Workspace
	AllWorkspaces bool
	// ArchivedView lists only archived sessions, from every workspace.
	ArchivedView bool
	// TasksView lists only the sessions plan tasks ran in, from every
	// workspace; the other views leave them out.
	TasksView bool
	// Tasks names the plan task each worker session ran, from the plan
	// registry.
	Tasks  map[domain.SessionID]planTaskRef
	Query  string
	Labels map[domain.SessionID]domain.SessionLabel
	// Renaming edits the selected session's title in place of the search.
	// RenameSelected marks the old title as selected, as F2 leaves it: the
	// first character typed replaces it. On 12 October 2026 F2 put the
	// cursor after the old title and typing appended to it.
	Renaming       bool
	RenameText     string
	RenameSelected bool
	// NewRow offers "＋ New session" above the sessions, and OnNew puts the
	// selection on it.
	NewRow bool
	OnNew  bool
	// Page is how many sessions the overlay shows at once, for PgUp and
	// PgDn; the root sets it from the screen's height.
	Page int
	// Deleting asks to confirm deleting the selected session: y deletes it,
	// any other key keeps it.
	Deleting bool
	// Held marks the sessions another console has open, which Enter does
	// not open; Open is the session this console shows. On 14 October 2026
	// neither was told apart from the rest of the list.
	Held map[domain.SessionID]bool
	Open domain.SessionID
	// content holds each session's prompts and replies, lowercased.
	content map[domain.SessionID]string
}

// PageSize is how many sessions fit in an overlay of this size: two rows
// per session and a blank, after the search row and up to three group
// labels, and the new-session row with its blank.
func (p SessionPicker) PageSize(height, width int) int {
	_, bodyHeight := OverlayBodySize(width, height)
	if p.ShowsNew() {
		return max(1, (bodyHeight-7)/3)
	}
	return max(1, (bodyHeight-5)/3)
}

// Paste adds pasted text to the filter, or to the new title while renaming,
// as one line.
func (p *SessionPicker) Paste(text string) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	if p.Renaming {
		if p.RenameSelected {
			p.RenameText, p.RenameSelected = "", false
		}
		p.insertRename(text)
		return
	}
	p.Query += text
	p.Selected, p.OnNew = 0, false
}

// ShowsNew reports whether the new-session row is listed: in the
// workspace scopes and while nothing is typed in the filter.
func (p SessionPicker) ShowsNew() bool {
	return p.NewRow && !p.ArchivedView && !p.TasksView && strings.TrimSpace(p.Query) == ""
}

// NewSelected reports the selection on the new-session row.
func (p SessionPicker) NewSelected() bool { return p.OnNew && p.ShowsNew() }

const (
	SessionRenameIntent  ControlIntent = "session-rename"
	SessionArchiveIntent ControlIntent = "session-archive"
	SessionDeleteIntent  ControlIntent = "session-delete"
)

// planTaskRef is the plan task a worker session ran.
type planTaskRef struct {
	Task, Plan string
	Status     domain.TaskStatus
}

// planTaskTitle reads a worker session's first prompt, its context pack:
// "Task <id> of plan <plan>, wave <n>: …". It finds the task sessions of a
// plan the registry no longer lists.
var planTaskTitle = regexp.MustCompile(`^Task ([a-z0-9][a-z0-9-]*) of plan ([a-z0-9][a-z0-9-]*), wave \d+`)

// planTaskSessions maps each plan task's worker session to its task.
func planTaskSessions(records []domain.PlanRecord) map[domain.SessionID]planTaskRef {
	tasks := map[domain.SessionID]planTaskRef{}
	for _, plan := range records {
		for _, task := range plan.Tasks {
			if task.Session != "" {
				tasks[task.Session] = planTaskRef{Task: task.ID, Plan: plan.ID, Status: task.Status}
			}
		}
	}
	return tasks
}

// planTask reports the plan task a session ran, if it is a worker session.
func (p SessionPicker) planTask(s domain.SessionSummary) (planTaskRef, bool) {
	if ref, ok := p.Tasks[s.ID]; ok {
		return ref, true
	}
	if match := planTaskTitle.FindStringSubmatch(string(s.Title)); match != nil {
		return planTaskRef{Task: match[1], Plan: match[2]}, true
	}
	return planTaskRef{}, false
}

// Title is the user's title for a session, or its first prompt. A plan
// task's session is named by its task and plan: on 10 October 2026 its
// title was the whole context pack, "Task buildinfo-short of plan
// add-a-func-dce953, wave 1: Add func Short() string … and cover it …".
func (p SessionPicker) Title(s domain.SessionSummary) string {
	if label := p.Labels[s.ID]; label.Title != "" {
		return singleLine(string(label.Title))
	}
	if ref, ok := p.planTask(s); ok {
		return singleLine("task " + ref.Task + " · plan " + ref.Plan)
	}
	return singleLine(string(s.Title))
}

func NewSessionPicker(items []domain.SessionSummary, workspace domain.Workspace) SessionPicker {
	sorted := append([]domain.SessionSummary(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt) })
	// Lowered once, not on every key of the filter.
	content := make(map[domain.SessionID]string, len(sorted))
	for _, s := range sorted {
		content[s.ID] = strings.ToLower(string(s.Text))
	}
	return SessionPicker{Items: sorted, Workspace: workspace, content: content}
}

// Visible is the filtered list the selection and clicks index into.
func (p SessionPicker) Visible() []domain.SessionSummary {
	query := strings.ToLower(strings.TrimSpace(p.Query))
	var visible []domain.SessionSummary
	for _, s := range p.Items {
		if s.MessageCount == 0 || p.Labels[s.ID].Archived != p.ArchivedView {
			continue
		}
		if _, task := p.planTask(s); !p.ArchivedView && task != p.TasksView {
			continue
		}
		if !p.ArchivedView && !p.TasksView && !p.AllWorkspaces && s.Workspace != p.Workspace {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(p.Title(s)), query) && !p.contentMatches(s, query) {
			continue
		}
		visible = append(visible, s)
	}
	return visible
}

// contentMatches reports a session whose prompts and replies contain the
// lowercased query. On 12 October 2026 the filter matched titles only.
func (p SessionPicker) contentMatches(s domain.SessionSummary, query string) bool {
	text, ok := p.content[s.ID]
	if !ok {
		text = strings.ToLower(string(s.Text))
	}
	return strings.Contains(text, query)
}

func (p SessionPicker) Current() (domain.SessionSummary, bool) {
	if p.NewSelected() {
		return domain.SessionSummary{}, false
	}
	visible := p.Visible()
	if p.Selected < 0 || p.Selected >= len(visible) {
		return domain.SessionSummary{}, false
	}
	return visible[p.Selected], true
}

// Key handles the picker's own keys. It returns an intent when the root has
// to persist something: a new title or an archive toggle.
func (p *SessionPicker) Key(key, text string) ControlIntent {
	if p.Deleting {
		p.Deleting = false
		if strings.ToLower(key) == "y" {
			return SessionDeleteIntent
		}
		return ""
	}
	if p.Renaming {
		selected := p.RenameSelected
		p.RenameSelected = false
		switch key {
		case "enter":
			p.Renaming = false
			return SessionRenameIntent
		case "esc":
			p.Renaming = false
		case "ctrl+u":
			p.RenameText = ""
		case "backspace":
			if selected {
				p.RenameText = ""
			} else if p.RenameText != "" {
				_, size := utf8.DecodeLastRuneInString(p.RenameText)
				p.RenameText = p.RenameText[:len(p.RenameText)-size]
			}
		default:
			if text == "" || strings.ContainsAny(text, "\r\n\t") {
				// Another key (an arrow, End) keeps the title and drops the
				// selection, so typing then appends.
				return ""
			}
			if selected {
				p.RenameText = ""
			}
			p.insertRename(text)
		}
		return ""
	}
	switch key {
	case "f2":
		if current, ok := p.Current(); ok {
			p.Renaming, p.RenameText, p.RenameSelected = true, p.Title(current), true
		}
		return ""
	case "ctrl+x":
		if _, ok := p.Current(); ok {
			return SessionArchiveIntent
		}
		return ""
	case "ctrl+delete":
		_, p.Deleting = p.Current()
		return ""
	}
	p.key(key, text)
	return ""
}

// insertRename adds typed or pasted text to the new title, up to the
// title's limit.
func (p *SessionPicker) insertRename(text string) {
	for _, r := range text {
		if utf8.RuneCountInString(p.RenameText) >= domain.MaxSessionTitleRunes {
			return
		}
		p.RenameText += string(r)
	}
}

func (p *SessionPicker) key(key, text string) bool {
	if !p.ShowsNew() {
		p.OnNew = false
	}
	switch key {
	case "up":
		if p.Selected == 0 && p.ShowsNew() {
			p.OnNew = true
		}
		p.Selected = max(0, p.Selected-1)
	case "down":
		if p.OnNew {
			p.OnNew = false
			break
		}
		p.Selected = min(max(0, len(p.Visible())-1), p.Selected+1)
	case "pgup":
		if p.Selected == 0 && p.ShowsNew() {
			p.OnNew = true
		}
		p.Selected = max(0, p.Selected-max(1, p.Page))
	case "pgdown":
		if p.OnNew {
			p.OnNew = false
			p.Selected = 0
		}
		p.Selected = min(max(0, len(p.Visible())-1), p.Selected+max(1, p.Page))
	case "home":
		p.Selected, p.OnNew = 0, p.ShowsNew()
	case "end":
		p.Selected, p.OnNew = max(0, len(p.Visible())-1), false
	case "tab":
		p.OnNew = false
		// This workspace → every workspace → plan tasks → archived → this
		// workspace.
		switch {
		case p.ArchivedView:
			p.ArchivedView, p.AllWorkspaces = false, false
		case p.TasksView:
			p.TasksView, p.ArchivedView = false, true
		case p.AllWorkspaces:
			p.TasksView = true
		default:
			p.AllWorkspaces = true
		}
		p.Selected = 0
	case "backspace":
		if p.Query != "" {
			_, size := utf8.DecodeLastRuneInString(p.Query)
			p.Query = p.Query[:len(p.Query)-size]
			p.Selected = 0
		}
	default:
		if text == "" || strings.ContainsAny(text, "\r\n\t") {
			return false
		}
		p.Query += text
		p.Selected = 0
	}
	return true
}

func (p SessionPicker) View(theme Theme, z *zone.Manager, prefix string, height, width int) string {
	return p.view(theme, z, prefix, height, width, time.Now())
}

func (p SessionPicker) view(theme Theme, z *zone.Manager, prefix string, height, width int, now time.Time) string {
	innerWidth, _ := OverlayBodySize(width, height)
	visible := p.Visible()
	var lines []string
	search := theme.Muted(theme.Icon("search")+" ") + p.Query
	if p.Query == "" {
		search = theme.Muted(theme.Icon("search") + " " + theme.T("sessions.searchHint"))
	}
	if current, ok := p.Current(); ok && p.Deleting {
		search = theme.Accent(theme.Tf("sessions.deleteConfirm", ansi.Truncate(p.Title(current), 40, "…")))
	}
	if p.Renaming {
		title := p.RenameText + theme.Accent("▏")
		if p.RenameSelected && p.RenameText != "" {
			title = theme.Selected(p.RenameText)
			if theme.Monochrome {
				// Without colour the selection is bracketed.
				title = "[" + p.RenameText + "]"
			}
		}
		search = theme.Accent(theme.T("sessions.renamePrompt")) + " " + title + theme.Muted("  "+theme.T("sessions.renameHint"))
	}
	lines = append(lines, search, "")
	page := p.PageSize(height, width)
	if p.ShowsNew() {
		plus := "＋"
		if theme.Monochrome || theme.Icons == domain.IconsASCII {
			plus = "+"
		}
		row := plus + " " + theme.T("sessions.new")
		switch {
		case !p.OnNew:
			row = "  " + row
		case theme.Monochrome:
			row = theme.Selected(row)
		default:
			row = theme.Selected(theme.Icon("user") + " " + row)
		}
		lines = append(lines, z.Mark(prefix+"session-new", row), "")
	}
	start := max(0, p.Selected-page+1)
	start = min(start, max(0, len(visible)-page))
	end := min(len(visible), start+page)
	group := ""
	for i := start; i < end; i++ {
		s := visible[i]
		if g := sessionGroup(s.UpdatedAt, now, theme); g != group {
			group = g
			lines = append(lines, theme.Muted(g))
		}
		title := p.Title(s)
		when := relativeTime(s.UpdatedAt, now, theme)
		// Two columns lead every row: the selection marker or blanks.
		body := innerWidth - 2
		head := ansi.Truncate(title, max(1, body-ansi.StringWidth(when)-2), "…")
		head += strings.Repeat(" ", max(2, body-ansi.StringWidth(head)-ansi.StringWidth(when))) + theme.Muted(when)
		switch {
		case i != p.Selected || p.NewSelected():
			head = "  " + head
		case theme.Monochrome:
			head = theme.Selected(head)
		default:
			head = theme.Selected(theme.Icon("user") + " " + head)
		}
		lines = append(lines, z.Mark(fmt.Sprintf("%ssession-%d", prefix, i), head))
		count := theme.Tn("sessions.messages", s.MessageCount, s.MessageCount)
		if s.MessageCount == 1 {
			count = theme.T("sessions.oneMessage")
		}
		meta := []string{singleLine(string(s.Model)), count}
		switch {
		case s.ID == p.Open:
			meta = append(meta, theme.T("sessions.openHere"))
		case p.Held[s.ID]:
			meta = append(meta, theme.T("sessions.openElsewhere"))
		}
		if s.Status == domain.StatusInterrupted || s.Status == domain.StatusApproval {
			meta = append(meta, theme.T("status."+string(s.Status)))
		}
		if ref, ok := p.planTask(s); ok && ref.Status != "" {
			meta = append(meta, theme.T("plans.task."+string(ref.Status)))
		}
		if query := strings.ToLower(strings.TrimSpace(p.Query)); query != "" && !strings.Contains(strings.ToLower(title), query) {
			meta = append(meta, theme.T("sessions.contentMatch"))
		}
		if s.Workspace != p.Workspace {
			// It opens only in its own workspace: Enter says how.
			meta = append([]string{theme.Tf("sessions.elsewhere", shortenHome(singleLine(string(s.Workspace))))}, meta...)
		} else if p.AllWorkspaces || p.ArchivedView || p.TasksView {
			meta = append(meta, shortenHome(singleLine(string(s.Workspace))))
		}
		if i == p.Selected && !p.NewSelected() {
			// What axlr-tui --session takes to resume it from a shell.
			meta = append(meta, theme.Tf("sessions.id", s.ID))
		}
		lines = append(lines, theme.Muted(ansi.Truncate("  "+strings.Join(meta, " · "), innerWidth, "…")))
	}
	if len(visible) == 0 {
		empty := theme.T("sessions.empty")
		switch {
		case p.ArchivedView:
			empty = theme.T("sessions.emptyArchived")
		case p.TasksView:
			empty = theme.T("sessions.emptyTasks")
		case !p.AllWorkspaces:
			empty = theme.T("sessions.emptyHere")
		}
		lines = append(lines, theme.Muted(empty))
	}
	scope := theme.T("sessions.scopeHere")
	switch {
	case p.ArchivedView:
		scope = theme.T("sessions.scopeArchived")
	case p.TasksView:
		scope = theme.T("sessions.scopeTasks")
	case p.AllWorkspaces:
		scope = theme.T("sessions.scopeAll")
	}
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	// Ctrl+X restores an archived session, so the archived scope says so.
	archive := theme.T("sessions.archiveVerb")
	if p.ArchivedView {
		archive = theme.T("sessions.archiveVerbArchived")
	}
	hint := theme.Tf("sessions.hint", theme.Tn("count.sessions", len(visible), len(visible)), scope, archive)
	return theme.Overlay(theme.T("sessions.title"), hint, strings.Join(lines, "\n"), footer, width, height)
}

func sessionGroup(t, now time.Time, theme Theme) string {
	day := func(x time.Time) time.Time {
		y, m, d := x.Local().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	}
	switch diff := day(now).Sub(day(t)); {
	case diff <= 0:
		return theme.T("sessions.today")
	case diff <= 24*time.Hour:
		return theme.T("sessions.yesterday")
	default:
		return theme.T("sessions.earlier")
	}
}

func relativeTime(t, now time.Time, theme Theme) string {
	switch d := now.Sub(t); {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return theme.T("sessions.justNow")
	case d < time.Hour:
		return theme.Tf("sessions.minutesAgo", int(d.Minutes()))
	case d < 24*time.Hour:
		return theme.Tf("sessions.hoursAgo", int(d.Hours()))
	case d < 48*time.Hour:
		return theme.T("sessions.oneDayAgo")
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		return theme.Tn("sessions.daysAgo", days, days)
	default:
		return t.Local().Format("2006-01-02")
	}
}
