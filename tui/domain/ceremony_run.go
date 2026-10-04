package domain

import (
	"errors"
	"slices"
)

// CheckCommand is a program and its arguments, run without a shell so the
// console repeats exactly what the user approved.
type CheckCommand struct {
	Program string
	Args    []string
}

func (c CheckCommand) Equal(other CheckCommand) bool {
	return c.Program == other.Program && slices.Equal(c.Args, other.Args)
}

func (c CheckCommand) IsZero() bool { return c.Program == "" }

// CeremonyRun is what a session knows about its live MADE instance. The
// console drives it; the model only does each step's work.
type CeremonyRun struct {
	Definition string
	Version    string
	Instance   string
	Step       string
	Iteration  int
	Fence      string
	// Check is the command the user approved for this instance; zero until a
	// step proposes one.
	Check CheckCommand
	// About is the exact scope used for initial recall and original MADE inputs.
	About string
	// Memory is the bounded wake text captured when the ceremony began.
	Memory string
	// BudgetBase is the turn's call count when the last step was accepted;
	// the per-turn call limit counts from it, so each step gets a full budget.
	BudgetBase int
	// Reminded is true once the console reminded the model that this step
	// is still open; it reminds once per claimed step.
	Reminded bool
}

func (r CeremonyRun) Validate() error {
	if r.Definition == "" || r.Version == "" || r.Instance == "" || r.Step == "" || r.Iteration < 1 || r.BudgetBase < 0 {
		return errors.New("ceremony run needs definition, version, instance, step and iteration")
	}
	return nil
}

func (r CeremonyRun) clone() CeremonyRun {
	r.Check.Args = append([]string(nil), r.Check.Args...)
	return r
}
