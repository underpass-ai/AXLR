package domain

import "slices"

// CheckCommand is a program and its arguments, run without a shell so the
// console repeats exactly what the user approved.
type CheckCommand struct {
	Program string
	Args    []string
	// MaxOutput raises the output cap for a console command whose output the
	// console parses (git status, gh pr view); the runner bounds it by the
	// runtime's hard limit. Zero keeps the cap of the model's checks. It is
	// never set on an approved command, so it is not persisted or compared.
	MaxOutput int
}

func (c CheckCommand) Equal(other CheckCommand) bool {
	return c.Program == other.Program && slices.Equal(c.Args, other.Args)
}

func (c CheckCommand) IsZero() bool { return c.Program == "" }
