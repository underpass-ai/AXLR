package application

// GuideResult is what kmp_guide gave the console: the agent and the
// context it opened.
type GuideResult struct {
	AgentID, ContextID string
}
