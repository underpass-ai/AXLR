package terminal

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionPicker lists saved sessions newest first. By default it shows the
// active workspace's sessions that have messages; Tab widens it to every
// workspace and typing filters by title.
type SessionPicker struct {
	Items         []domain.SessionSummary
	Selected      int
	Workspace     domain.Workspace
	AllWorkspaces bool
	Query         string
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
		if s.MessageCount == 0 || !p.AllWorkspaces && s.Workspace != p.Workspace {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(singleLine(string(s.Title))), query) {
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

// Key handles the picker's own keys and reports whether it consumed them.
func (p *SessionPicker) Key(key, text string) bool {
	switch key {
	case "up":
		p.Selected = max(0, p.Selected-1)
	case "down":
		p.Selected = min(max(0, len(p.Visible())-1), p.Selected+1)
	case "tab":
		p.AllWorkspaces = !p.AllWorkspaces
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
		title := singleLine(string(s.Title))
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
		if p.AllWorkspaces {
			meta = append(meta, shortenHome(singleLine(string(s.Workspace))))
		}
		lines = append(lines, theme.Muted(ansi.Truncate("  "+strings.Join(meta, " · "), innerWidth, "…")))
	}
	if len(visible) == 0 {
		empty := theme.T("sessions.empty")
		if !p.AllWorkspaces {
			empty = theme.T("sessions.emptyHere")
		}
		lines = append(lines, theme.Muted(empty))
	}
	scope := theme.T("sessions.scopeHere")
	if p.AllWorkspaces {
		scope = theme.T("sessions.scopeAll")
	}
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	return theme.Overlay(theme.T("sessions.title"), theme.Tf("sessions.hint", len(visible), scope), strings.Join(lines, "\n"), footer, width, height)
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
