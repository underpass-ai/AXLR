package storage

import "github.com/underpass-ai/AXLR/tui/domain"

// consoleNotes is the part of a session's state <id>.console holds.
type consoleNotes struct {
	StoppedCeremony string
	Chosen          domain.CeremonyChoice
	Lines           []domain.ConsoleLine
}

func (n consoleNotes) empty() bool {
	return n.StoppedCeremony == "" && n.Chosen == (domain.CeremonyChoice{}) && len(n.Lines) == 0
}

func notesOf(state domain.SessionState) consoleNotes {
	return consoleNotes{StoppedCeremony: state.StoppedCeremony, Chosen: state.ChosenCeremony, Lines: state.ConsoleLines}
}

func (n consoleNotes) apply(state *domain.SessionState) {
	state.StoppedCeremony = n.StoppedCeremony
	state.ChosenCeremony = n.Chosen
	state.ConsoleLines = n.Lines
}
