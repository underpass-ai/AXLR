package terminal

import (
	"fmt"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// maxTurnLog is how many finished turns /usage lists.
const maxTurnLog = 10

// turnUsage is what one turn run in this console used: the difference of
// the session's ledger between the prompt and the turn's end, since the
// ledger keeps totals, not requests. On 12 October 2026 the cost of a turn
// was only in the live footer, gone with the next prompt.
type turnUsage struct {
	// At is when the turn began and Prompt the first words of the prompt
	// that began it: on 14 October 2026 /usage listed a turn begun at
	// 15:46 as "15:49 · 8 requests", its end, and nothing said which.
	At        time.Time
	Prompt    string
	Requests  int
	Tokens    int
	Cost      float64
	CostKnown bool
	Elapsed   time.Duration
}

// finishTurnUsage records the turn that just ended after messages
// messages, when it made a request.
func (m *AppModel) finishTurnUsage(messages int, elapsed time.Duration) {
	l := m.cost.ledger
	usage := turnUsage{At: time.Now().Add(-elapsed), Prompt: turnPrompt(m.Header.State.Messages, messages), Requests: l.Requests - m.cost.turnAll, Tokens: l.PromptTokens + l.CompletionTokens - m.cost.turnTokens, Elapsed: elapsed}
	if usage.Requests <= 0 {
		return
	}
	if l.CostRequests > m.cost.turnRequests {
		usage.Cost, usage.CostKnown = l.Cost-m.cost.turnCost, true
	}
	if m.turnCosts == nil {
		m.turnCosts = map[int]turnUsage{}
	}
	m.turnCosts[messages] = usage
	m.turnLog = append(m.turnLog, usage)
	if len(m.turnLog) > maxTurnLog {
		m.turnLog = m.turnLog[len(m.turnLog)-maxTurnLog:]
	}
}

// turnPromptRunes bounds the prompt's words on a /usage line.
const turnPromptRunes = 32

// turnPrompt is the first words of the person's last prompt among the
// first messages: its first line, whitespace collapsed, cut at a word.
func turnPrompt(messages []root.Message, end int) string {
	for i := min(end, len(messages)) - 1; i >= 0; i-- {
		content := messages[i].Content
		if messages[i].Role != root.RoleUser || strings.HasPrefix(string(content), "[AXLR") || domain.BudgetNote(content) {
			continue
		}
		typed, _ := splitConsoleNotes(content)
		line := ""
		for _, candidate := range strings.Split(typed, "\n") {
			if line = strings.Join(strings.Fields(candidate), " "); line != "" {
				break
			}
		}
		runes := []rune(Sanitize(line))
		if len(runes) <= turnPromptRunes {
			return string(runes)
		}
		cut := turnPromptRunes - 1
		for j := cut; j > turnPromptRunes/2; j-- {
			if runes[j] == ' ' {
				cut = j
				break
			}
		}
		return strings.TrimSpace(string(runes[:cut])) + "…"
	}
	return ""
}

// turnCostLine is the dim "· $0.0046 · 27k tok" under a finished turn, ""
// when neither is known.
func turnCostLine(u turnUsage) string {
	var parts []string
	if u.CostKnown {
		parts = append(parts, FormatUSD(u.Cost))
	}
	if u.Tokens > 0 {
		parts = append(parts, shortTokens(u.Tokens)+" tok")
	}
	if len(parts) == 0 {
		return ""
	}
	return "· " + strings.Join(parts, " · ")
}

// turnCostLines are the lines the transcript draws under finished turns,
// by the number of messages each turn ended after.
func (m AppModel) turnCostLines() map[int]string {
	lines := map[int]string{}
	for at, usage := range m.turnCosts {
		if line := turnCostLine(usage); line != "" {
			lines[at] = line
		}
	}
	return lines
}

// shortTokens is formatTokens without a trailing ".0": 980, 9.5k, 27k,
// 1.2M.
func shortTokens(n int) string {
	text := formatTokens(n)
	if n >= 10_000 && n < 1_000_000 {
		text = fmt.Sprintf("%dk", (n+500)/1000)
	}
	return strings.Replace(text, ".0", "", 1)
}

// turnLogLines are /usage's last turns, newest first.
func turnLogLines(turns []turnUsage, theme Theme) []string {
	if len(turns) == 0 {
		return nil
	}
	lines := []string{theme.T("usage.turns")}
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		cost := "—"
		if t.CostKnown {
			cost = FormatUSD(t.Cost)
		}
		when := formatClock(t.At, transcriptNow())
		if t.Prompt != "" {
			when += " · " + t.Prompt
		}
		lines = append(lines, theme.Tn("usage.turn", t.Requests, when, t.Requests, shortTokens(t.Tokens), cost, formatElapsed(t.Elapsed)))
	}
	return lines
}

// footerTokens is the footer's "27k/64k tok": the last request's prompt
// tokens and the prompt the console lets the session's requests reach,
// when it knows it.
func (m AppModel) footerTokens() string {
	text := shortTokens(m.tokens)
	if m.deps.PromptCeiling != nil && m.Header.State.Model != "" {
		if ceiling := m.deps.PromptCeiling(m.Header.State.Model); ceiling > 0 {
			text += "/" + shortTokens(ceiling)
		}
	}
	return m.Theme.Tf("footer.tokens", text)
}
