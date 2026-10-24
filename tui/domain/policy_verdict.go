package domain

// PolicyVerdict is what an execution's policy says of one resolved call.
type PolicyVerdict int

const (
	// PolicyAllow lets the call through to the rest of the chain: the
	// run's approval level, the settings' rules, the work mode and the
	// destructive list.
	PolicyAllow PolicyVerdict = iota
	// PolicyAsk lets the call through with a decision: it never runs
	// without a card, autonomy included.
	PolicyAsk
	// PolicyForbid refuses the call outside the model, before any card.
	PolicyForbid
)
