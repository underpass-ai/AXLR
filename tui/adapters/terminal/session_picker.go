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
	Renaming   bool
	RenameText string
}

const (
	SessionRenameIntent  ControlIntent = "session-rename"
	SessionArchiveIntent ControlIntent = "session-archive"
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
	return SessionPicker{Items: sorted, Workspace: workspace}
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
		if query != "" && !strings.Contains(strings.ToLower(p.Title(s)), query) {
			continue
		}
		visible = append(visible, s)
	}
	return visible
}

func (p SessionPicker) Current() (domain.SessionSummary, bool) {
	visible := p.Visible()
	if p.Selected < 0 || p.Selected >= len(visible) {
		return domain.SessionSummary{}, false
	}
	return visible[p.Selected], true
}

// Key handles the picker's own keys. It returns an intent when the root has
// to persist something: a new title or an archive toggle.
func (p *SessionPicker) Key(key, text string) ControlIntent {
	if p.Renaming {
		switch key {
		case "enter":
			p.Renaming = false
			return SessionRenameIntent
		case "esc":
			p.Renaming = false
		case "backspace":
			if p.RenameText != "" {
				_, size := utf8.DecodeLastRuneInString(p.RenameText)
				p.RenameText = p.RenameText[:len(p.RenameText)-size]
			}
		default:
			if text != "" && !strings.ContainsAny(text, "\r\n\t") && utf8.RuneCountInString(p.RenameText) < domain.MaxSessionTitleRunes {
				p.RenameText += text
			}
		}
		return ""
	}
	switch key {
	case "f2":
		if current, ok := p.Current(); ok {
			p.Renaming, p.RenameText = true, p.Title(current)
		}
		return ""
	case "ctrl+x":
		if _, ok := p.Current(); ok {
			return SessionArchiveIntent
		}
		return ""
	}
	p.key(key, text)
	return ""
}

func (p *SessionPicker) key(key, text string) bool {
	switch key {
	case "up":
		p.Selected = max(0, p.Selected-1)
	case "down":
		p.Selected = min(max(0, len(p.Visible())-1), p.Selected+1)
	case "tab":
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
	innerWidth, bodyHeight := OverlayBodySize(width, height)
	visible := p.Visible()
	var lines []string
	search := theme.Muted(theme.Icon("search")+" ") + p.Query
	if p.Query == "" {
		search = theme.Muted(theme.Icon("search") + " " + theme.T("sessions.searchHint"))
	}
	if p.Renaming {
		search = theme.Accent(theme.T("sessions.renamePrompt")) + " " + p.RenameText + theme.Accent("▏") + theme.Muted("  "+theme.T("sessions.renameHint"))
	}
	lines = append(lines, search, "")
	// Two rows per session plus a blank, after the search row and up to three group labels.
	page := max(1, (bodyHeight-5)/3)
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
		case i != p.Selected:
			head = "  " + head
		case theme.Monochrome:
			head = theme.Selected(head)
		default:
			head = theme.Selected(theme.Icon("user") + " " + head)
		}
		lines = append(lines, z.Mark(fmt.Sprintf("%ssession-%d", prefix, i), head))
		count := theme.Tf("sessions.messages", s.MessageCount)
		if s.MessageCount == 1 {
			count = theme.T("sessions.oneMessage")
		}
		meta := []string{singleLine(string(s.Model)), count}
		if s.Status == domain.StatusInterrupted || s.Status == domain.StatusApproval {
			meta = append(meta, theme.T("status."+string(s.Status)))
		}
		if ref, ok := p.planTask(s); ok && ref.Status != "" {
			meta = append(meta, theme.T("plans.task."+string(ref.Status)))
		}
		if p.AllWorkspaces || p.ArchivedView || p.TasksView {
			meta = append(meta, shortenHome(singleLine(string(s.Workspace))))
		}
		if i == p.Selected {
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
	hint := theme.Tf("sessions.hint", len(visible), scope)
	if len(visible) == 1 {
		hint = theme.Tf("sessions.hintOne", scope)
	}
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
		return theme.Tf("sessions.daysAgo", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}
