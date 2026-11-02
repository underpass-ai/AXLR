package domain

import (
	"fmt"
	"slices"
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
	// Unmetered marks a request that was cancelled or failed after the
	// provider may have started billing it, and reported no usage: its
	// tokens are estimates (EstimatedTokens) and its cost is unknown.
	Unmetered bool
	// GenerationID is OpenRouter's id of an unmetered request, by which its
	// exact cost can be looked up later; Ended is when it ended.
	GenerationID string
	Ended        time.Time
}

// EstimatedTokens is how many tokens bytes hold at hundredths of a byte per
// token, rounded up; the default prompt ratio when hundredths is not
// positive. It sizes an unmetered request.
func EstimatedTokens(bytes, hundredths int) int {
	if bytes <= 0 {
		return 0
	}
	if hundredths <= 0 {
		hundredths = DefaultBytesPerTokenHundredths
	}
	return (bytes*100 + hundredths - 1) / hundredths
}

// UsageLedger is a session's running account of its model requests:
// totals, subtotals by provider and latency aggregates, so its size does
// not grow with the session. The one list it keeps, Pending, is bounded
// and empties as each request in it is settled or given up. A 198-request
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
	// Unmetered counts the requests cancelled or failed without a usage
	// report, which the provider may still have billed: their tokens are
	// estimates and their cost is missing from Cost. On 12 October 2026 an
	// essay cancelled after ten seconds of streaming left the session's
	// cost where it was.
	Unmetered int
	// Pending are the unmetered OpenRouter requests whose exact cost may
	// still be looked up (Settle), at most MaxPendingGenerations.
	Pending   []PendingGeneration
	Providers map[string]ProviderUsage
	FirstByte LatencyStats
	Total     LatencyStats
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
	if s.Unmetered {
		l.Unmetered++
	}
	if l.Providers == nil {
		l.Providers = map[string]ProviderUsage{}
	}
	name := s.Provider
	if _, seen := l.Providers[name]; !seen && len(l.Providers) >= MaxLedgerProviders {
		name = OtherProvider
	}
	if s.Unmetered && s.GenerationID != "" && len(l.Pending) < MaxPendingGenerations {
		l.Pending = append(l.Pending, PendingGeneration{ID: s.GenerationID, Provider: name, PromptTokens: s.PromptTokens, CompletionTokens: s.CompletionTokens, Ended: s.Ended})
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
	return s.CostKnown && l.warn(maxSessionUSD)
}

// warn arms the warning at 80 % of the limit once: it reports whether the
// cost passed it while the session had not been warned.
func (l *UsageLedger) warn(maxSessionUSD float64) bool {
	if l.Warned || !l.Near(maxSessionUSD) {
		return false
	}
	l.Warned = true
	return true
}

// Settle replaces the estimate of the pending request id with what
// OpenRouter reported of it: its tokens, when reported, take the
// estimated ones' place, its cost joins the known cost and it is no
// longer unmetered. It reports whether the request was pending, and
// whether its cost took the session past 80 % of the limit as Record
// does; an unknown id changes nothing.
func (l *UsageLedger) Settle(id string, exact GenerationCost, maxSessionUSD float64) (settled, warned bool) {
	index := l.pendingIndex(id)
	if index < 0 || exact.Cost < 0 {
		return false, false
	}
	pending := l.Pending[index]
	l.Pending = append(l.Pending[:index:index], l.Pending[index+1:]...)
	l.Unmetered = max(0, l.Unmetered-1)
	prompt, completion := pending.PromptTokens, pending.CompletionTokens
	if exact.TokensKnown {
		prompt, completion = max(0, exact.PromptTokens), max(0, exact.CompletionTokens)
	}
	l.PromptTokens = max(0, l.PromptTokens-pending.PromptTokens+prompt)
	l.CompletionTokens = max(0, l.CompletionTokens-pending.CompletionTokens+completion)
	l.Cost += exact.Cost
	l.CostRequests++
	if provider, ok := l.Providers[pending.Provider]; ok {
		provider.PromptTokens = max(0, provider.PromptTokens-pending.PromptTokens+prompt)
		provider.CompletionTokens = max(0, provider.CompletionTokens-pending.CompletionTokens+completion)
		provider.Cost += exact.Cost
		provider.CostRequests++
		l.Providers[pending.Provider] = provider
	}
	return true, l.warn(maxSessionUSD)
}

// Missed counts a lookup of the pending request id that brought no cost,
// at now. A request asked for MaxGenerationLookups times, or pending for
// MaxGenerationWait, is given up: it keeps its estimate and stays
// unmetered.
func (l *UsageLedger) Missed(id string, now time.Time) {
	if index := l.pendingIndex(id); index >= 0 {
		// A copy of the ledger, one told to the console, keeps its list.
		l.Pending = slices.Clone(l.Pending)
		l.Pending[index].Lookups++
		l.Pending[index].Tried = now
	}
	l.Expire(now)
}

// Expire gives up the pending requests past their lookups or their wait.
func (l *UsageLedger) Expire(now time.Time) {
	var kept []PendingGeneration
	for _, pending := range l.Pending {
		if !pending.Expired(now) {
			kept = append(kept, pending)
		}
	}
	l.Pending = kept
}

func (l UsageLedger) pendingIndex(id string) int {
	for index, pending := range l.Pending {
		if pending.ID == id {
			return index
		}
	}
	return -1
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
