package domain

import (
	"fmt"
	"time"
)

// UsageSample is what one model request consumed, as its provider reported
// it, and how long it took.
type UsageSample struct {
	// Provider names who served the request: OpenRouter's upstream, or the
	// model for a server that does not say, such as a local one.
	Provider         string
	PromptTokens     int
	CachedTokens     int
	CacheWriteTokens int
	CompletionTokens int
	ReasoningTokens  int
	// Cost is in US dollars and counts only when CostKnown: a local
	// server's request is not free but unknown.
	Cost      float64
	CostKnown bool
	// FirstByte is the time to the provider's first activity or text, zero
	// when none was seen; Total is the whole request.
	FirstByte time.Duration
	Total     time.Duration
}

// UsageLedger is a session's running account of its model requests:
// totals, subtotals by provider and latency aggregates, never a list of
// requests, so its size does not grow with the session. A 198-request
// Claude Haiku session cost $14.13 on 8 October 2026 before anyone noticed.
type UsageLedger struct {
	Requests         int
	PromptTokens     int
	CachedTokens     int
	CacheWriteTokens int
	CompletionTokens int
	ReasoningTokens  int
	// Cost sums the CostRequests that reported a cost.
	Cost         float64
	CostRequests int
	Providers    map[string]ProviderUsage
	FirstByte    LatencyStats
	Total        LatencyStats
	// Raised is what the person allowed beyond max_session_usd for this
	// session, in dollars.
	Raised float64
	// Warned records the warning at 80 % of the current limit; a raise
	// arms it again.
	Warned bool
}

// ProviderUsage is one provider's share of a ledger.
type ProviderUsage struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	CostRequests     int
}

// LatencyStats aggregates durations without keeping them.
type LatencyStats struct {
	Count int
	Sum   time.Duration
	Max   time.Duration
}

func (s LatencyStats) Average() time.Duration {
	if s.Count == 0 {
		return 0
	}
	return s.Sum / time.Duration(s.Count)
}

func (s *LatencyStats) add(d time.Duration) {
	if d <= 0 {
		return
	}
	s.Count++
	s.Sum += d
	s.Max = max(s.Max, d)
}

const (
	// MaxLedgerProviders bounds the subtotals; providers past it share
	// OtherProvider.
	MaxLedgerProviders = 16
	OtherProvider      = "other"
	// BudgetWarningShare is the share of the limit past which the session
	// is warned, once.
	BudgetWarningShare = 0.8
)

// Limit is the session's budget in dollars for a configured
// max_session_usd: that amount plus what the person raised; zero, no limit.
func (l UsageLedger) Limit(maxSessionUSD float64) float64 {
	if maxSessionUSD <= 0 {
		return 0
	}
	return maxSessionUSD + l.Raised
}

// Exhausted reports whether the known cost reached the limit.
func (l UsageLedger) Exhausted(maxSessionUSD float64) bool {
	limit := l.Limit(maxSessionUSD)
	return limit > 0 && l.Cost >= limit
}

// Near reports whether the known cost passed 80 % of the limit.
func (l UsageLedger) Near(maxSessionUSD float64) bool {
	limit := l.Limit(maxSessionUSD)
	return limit > 0 && l.Cost >= BudgetWarningShare*limit
}

// Record adds one request. It reports whether this request took the known
// cost past 80 % of the limit while the session had not been warned; a
// request without a cost never counts towards the limit nor warns.
func (l *UsageLedger) Record(s UsageSample, maxSessionUSD float64) bool {
	l.Requests++
	l.PromptTokens += s.PromptTokens
	l.CachedTokens += s.CachedTokens
	l.CacheWriteTokens += s.CacheWriteTokens
	l.CompletionTokens += s.CompletionTokens
	l.ReasoningTokens += s.ReasoningTokens
	l.FirstByte.add(s.FirstByte)
	l.Total.add(s.Total)
	if l.Providers == nil {
		l.Providers = map[string]ProviderUsage{}
	}
	name := s.Provider
	if _, seen := l.Providers[name]; !seen && len(l.Providers) >= MaxLedgerProviders {
		name = OtherProvider
	}
	provider := l.Providers[name]
	provider.Requests++
	provider.PromptTokens += s.PromptTokens
	provider.CompletionTokens += s.CompletionTokens
	// A negative cost is no cost the person paid; it must not lower the
	// total the limit is checked against.
	s.CostKnown = s.CostKnown && s.Cost >= 0
	if s.CostKnown {
		provider.Cost += s.Cost
		provider.CostRequests++
		l.Cost += s.Cost
		l.CostRequests++
	}
	l.Providers[name] = provider
	if !s.CostKnown || l.Warned || !l.Near(maxSessionUSD) {
		return false
	}
	l.Warned = true
	return true
}

// Raise allows another maxSessionUSD for this session.
func (l *UsageLedger) Raise(maxSessionUSD float64) {
	if maxSessionUSD > 0 {
		l.Raised += maxSessionUSD
		l.Warned = false
	}
}

// SessionBudgetError refuses a model request once the session's known cost
// reached its limit (max_session_usd plus what was raised). Raise is what
// the person may allow again from /usage: another max_session_usd.
type SessionBudgetError struct {
	Limit, Spent, Raise float64
}

func (e *SessionBudgetError) Error() string {
	return fmt.Sprintf("this session reached its $%.2f budget (max_session_usd) with $%.2f spent", e.Limit, e.Spent)
}
