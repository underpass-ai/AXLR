package application

// MissingDefinition is a pinned MADE definition the connected store does not
// publish yet.
type MissingDefinition struct {
	Name, Version string
}
