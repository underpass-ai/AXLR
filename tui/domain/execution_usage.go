package domain

// ExecutionUsage is what an execution consumed, summed over the sessions
// it spanned: the ledger's totals without its latency or provider detail.
type ExecutionUsage struct {
	Requests         int
	PromptTokens     int
	CachedTokens     int
	CacheWriteTokens int
	CompletionTokens int
	ReasoningTokens  int
	// CostUSD sums the requests that reported a cost, CostRequests of them;
	// Unmetered counts the requests that reported none, which the provider
	// may still have billed.
	CostUSD      float64
	CostRequests int
	Unmetered    int
}

// Add sums one session's ledger into the usage.
func (u *ExecutionUsage) Add(ledger UsageLedger) {
	u.Requests += ledger.Requests
	u.PromptTokens += ledger.PromptTokens
	u.CachedTokens += ledger.CachedTokens
	u.CacheWriteTokens += ledger.CacheWriteTokens
	u.CompletionTokens += ledger.CompletionTokens
	u.ReasoningTokens += ledger.ReasoningTokens
	u.CostUSD += ledger.Cost
	u.CostRequests += ledger.CostRequests
	u.Unmetered += ledger.Unmetered
}

// addRequest sums one request of a receipt: its cost when it reported one,
// an unmetered request when its tokens are estimates or it never ended.
func (u *ExecutionUsage) addRequest(request ReceiptRequest) {
	sample := request.Usage
	u.Requests++
	u.PromptTokens += sample.PromptTokens
	u.CachedTokens += sample.CachedTokens
	u.CacheWriteTokens += sample.CacheWriteTokens
	u.CompletionTokens += sample.CompletionTokens
	u.ReasoningTokens += sample.ReasoningTokens
	if sample.CostKnown && sample.Cost >= 0 {
		u.CostUSD += sample.Cost
		u.CostRequests++
	}
	if sample.Unmetered {
		u.Unmetered++
	}
}
