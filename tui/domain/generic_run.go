package domain

// GenericRun is what the generic loop keeps of a definition: each step's
// prompt and the terminal states.
type GenericRun struct {
	Prompts  map[string]string
	Terminal []string
}
