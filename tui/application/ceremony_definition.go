package application

// CeremonyDefinition is what the generic driver reads from a definition.
type CeremonyDefinition struct {
	Name, Version, Description string
	// Inputs are the required inputs; Initial the initial state; Terminal
	// the terminal states; Steps the steps with their states; Prompts each
	// step's prompt; Transitions the triggers leaving each state.
	Inputs      []string
	Initial     string
	Terminal    []string
	Steps       []DefinitionStep
	Prompts     map[string]string
	Transitions map[string][]string
}
