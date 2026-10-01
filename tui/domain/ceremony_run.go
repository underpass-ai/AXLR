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
	// About is the KMP anchor for this session's ceremonies (ws:<session id>).
	About string
	// Memory is the bounded wake text captured when the ceremony began.
	Memory string
}

func (r CeremonyRun) Validate() error {
	if r.Definition == "" || r.Version == "" || r.Instance == "" || r.Step == "" || r.Iteration < 1 {
		return errors.New("ceremony run needs definition, version, instance, step and iteration")
	}
	return nil
}

func (r CeremonyRun) clone() CeremonyRun {
	r.Check.Args = append([]string(nil), r.Check.Args...)
	return r
}
