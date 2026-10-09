package terminal

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// costView is what the footer and /usage show of the session's usage
// ledger. turnCost and turnRequests are the ledger's known cost and costed
// requests when the person last sent a prompt, so the turn's cost is the
// difference.
type costView struct {
	ledger       domain.UsageLedger
	turnCost     float64
	turnRequests int
}

func (c *costView) beginTurn() {
	c.turnCost, c.turnRequests = c.ledger.Cost, c.ledger.CostRequests
}

// loadUsage reads the ledger of the session on screen, so the session's
// cost survives a restart; a ledger that cannot be read shows no cost.
func (m AppModel) loadUsage() AppModel {
	m.cost = costView{}
	if m.deps.Usage != nil && m.Header.State.ID != "" {
		if ledger, err := m.deps.Usage.LoadUsage(m.lifetime.ctx, m.Header.State.ID); err == nil {
			m.cost.ledger = ledger
		}
	}
	m.cost.beginTurn()
	return m
}

// observeUsage takes the ledger a request left; warn marks the request that
// passed 80 % of the budget.
func (m AppModel) observeUsage(ledger domain.UsageLedger, warn bool) AppModel {
	m.cost.ledger = ledger
	if warn {
		m.Status.Notice = m.Theme.Tf("notice.budgetWarning", formatUSD(ledger.Cost), formatBudget(ledger.Limit(m.deps.MaxSessionUSD)))
	}
	return m.refreshUsagePanel()
}

// costParts is the footer's turn and session cost; absent while no request
// reported one.
func (m AppModel) costParts() []string {
	var parts []string
	ledger := m.cost.ledger
	if ledger.CostRequests > m.cost.turnRequests {
		parts = append(parts, m.Theme.Tf("footer.costTurn", formatUSD(ledger.Cost-m.cost.turnCost)))
	}
	if ledger.CostRequests > 0 {
		parts = append(parts, m.Theme.Tf("footer.costSession", formatUSD(ledger.Cost)))
	}
	return parts
}

// formatUSD shows what was spent, with four decimals below a cent:
// $0.0031, $0.21, $14.13.
func formatUSD(v float64) string {
	if v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// formatBudget shows a limit or what is left of it in cents, $0.00 when
// nothing is left, and like formatUSD below a cent.
func formatBudget(v float64) string {
	if v > 0 {
		return formatUSD(v)
	}
	return "$0.00"
}

// budgetErrorText says, in the person's language, how a session refused at
// its budget goes on; other errors keep their text.
func (m AppModel) budgetErrorText(err error, text string) string {
	var budget *domain.SessionBudgetError
	if !errors.As(err, &budget) {
		return text
	}
	return strings.Replace(text, budget.Error(), m.Theme.Tf("error.sessionBudget", formatBudget(budget.Limit), formatBudget(budget.Raise)), 1)
}

// canRaise reports whether + may allow another max_session_usd: a budget
// is set and the session passed 80 % of it.
func (m AppModel) canRaise() bool {
	return m.deps.Usage != nil && m.Header.State.ID != "" && m.cost.ledger.Near(m.deps.MaxSessionUSD)
}

func (m AppModel) openUsagePanel() AppModel {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(usageContent(m.cost.ledger, m.deps.MaxSessionUSD, m.Theme))
	m.Info.Viewport.GotoTop()
	m.overlay = "usage"
	return m
}

func (m AppModel) refreshUsagePanel() AppModel {
	if m.overlay != "usage" {
		return m
	}
	offset := m.Info.Viewport.YOffset()
	m.Info.SetContent(usageContent(m.cost.ledger, m.deps.MaxSessionUSD, m.Theme))
	m.Info.Viewport.SetYOffset(offset)
	return m
}

// usageKey raises this session's limit on +, while the budget is near or
// spent, and scrolls otherwise.
func (m AppModel) usageKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	if k.String() == "+" && m.canRaise() {
		limit := m.deps.MaxSessionUSD
		ledger, err := m.deps.Usage.UpdateUsage(m.lifetime.ctx, m.Header.State.ID, func(l *domain.UsageLedger) { l.Raise(limit) })
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		m.cost.ledger = ledger
		m.Status.Notice = m.Theme.Tf("usage.raised", formatBudget(ledger.Limit(limit)))
		return m.refreshUsagePanel(), nil
	}
	m.Info.Viewport, _ = m.Info.Viewport.Update(k)
	return m, nil
}

func (m AppModel) usagePanelView() string {
	subtitle := m.Theme.T("usage.subtitle")
	if m.canRaise() {
		subtitle = m.Theme.Tf("usage.subtitleRaise", formatBudget(m.deps.MaxSessionUSD))
	}
	return m.Theme.Overlay(m.Theme.T("usage.title"), subtitle, m.Info.View(), m.zones.Mark(m.prefix+"close", "["+m.Theme.T("common.close")+"]"), m.Layout.Width, m.Layout.Height-1)
}

// usageContent is the /usage panel: requests, tokens, cost in total and by
// provider (most expensive first), latency and the budget.
func usageContent(l domain.UsageLedger, maxSessionUSD float64, theme Theme) string {
	if l.Requests == 0 {
		return theme.T("usage.empty") + "\n\n" + budgetLines(l, maxSessionUSD, theme)
	}
	lines := []string{
		theme.Tf("usage.requests", l.Requests, l.CostRequests),
		theme.Tf("usage.prompt", formatTokens(l.PromptTokens), formatTokens(l.CachedTokens), formatTokens(l.CacheWriteTokens)),
		theme.Tf("usage.completion", formatTokens(l.CompletionTokens), formatTokens(l.ReasoningTokens)),
		"",
	}
	if l.CostRequests > 0 {
		lines = append(lines, theme.Tf("usage.cost", formatUSD(l.Cost)))
	} else {
		lines = append(lines, theme.T("usage.costUnknown"))
	}
	names := make([]string, 0, len(l.Providers))
	for name := range l.Providers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := l.Providers[names[i]], l.Providers[names[j]]
		if a.Cost != b.Cost {
			return a.Cost > b.Cost
		}
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return names[i] < names[j]
	})
	if len(names) > 0 {
		lines = append(lines, theme.T("usage.providers"))
	}
	for _, name := range names {
		p := l.Providers[name]
		cost := "—"
		if p.CostRequests > 0 {
			cost = formatUSD(p.Cost)
		}
		lines = append(lines, theme.Tf("usage.provider", singleLine(name), p.Requests, cost))
	}
	lines = append(lines, "",
		theme.Tf("usage.firstByte", formatLatency(l.FirstByte.Average(), l.FirstByte.Count), formatLatency(l.FirstByte.Max, l.FirstByte.Count)),
		theme.Tf("usage.total", formatLatency(l.Total.Average(), l.Total.Count), formatLatency(l.Total.Max, l.Total.Count)),
		"", budgetLines(l, maxSessionUSD, theme))
	return strings.Join(lines, "\n")
}

func budgetLines(l domain.UsageLedger, maxSessionUSD float64, theme Theme) string {
	limit := l.Limit(maxSessionUSD)
	if limit <= 0 {
		return theme.T("usage.noBudget")
	}
	left := formatBudget(max(0, limit-l.Cost))
	line := theme.Tf("usage.budget", formatBudget(limit), left)
	if l.Raised > 0 {
		line = theme.Tf("usage.budgetRaised", formatBudget(limit), formatBudget(l.Raised), left)
	}
	switch {
	case l.Exhausted(maxSessionUSD):
		line += "\n" + theme.Tf("usage.spent", formatBudget(maxSessionUSD))
	case l.Near(maxSessionUSD):
		line += "\n" + theme.Tf("usage.near", formatBudget(maxSessionUSD))
	}
	return line
}

// formatLatency is "—" without a measurement, milliseconds under a second
// and seconds with a decimal above.
func formatLatency(d time.Duration, count int) string {
	switch {
	case count == 0:
		return "—"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}
