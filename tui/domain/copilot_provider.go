package domain

// CopilotProvider is the provider the Copilot adapter names for every
// request. A Copilot subscription pays its requests in credits, not
// dollars: they report no cost, the ledger counts them under this name,
// and max_session_usd cannot limit them.
const CopilotProvider = "GitHub Copilot"

// CopilotRequests counts the session's requests to a Copilot model, which
// no USD cost covers.
func (l UsageLedger) CopilotRequests() int {
	return l.Providers[CopilotProvider].Requests
}
