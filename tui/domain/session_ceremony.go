package domain

import "errors"

// Ceremony returns a copy of the live ceremony run, or false when none runs.
func (s Session) Ceremony() (CeremonyRun, bool) {
	if s.state.Ceremony == nil {
		return CeremonyRun{}, false
	}
	return s.state.Ceremony.clone(), true
}

// SetCeremony records the console's progress through a ceremony. The console
// changes it while a turn runs, so it is not gated like SetMode.
func (s *Session) SetCeremony(run CeremonyRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	copied := run.clone()
	s.state.Ceremony = &copied
	return nil
}

// FinishCeremony closes a ceremony that reached a terminal state and returns
// the session to normal mode, even inside the turn that finished it: MADE,
// not the user, decided the ceremony is over.
func (s *Session) FinishCeremony() {
	if s.state.Ceremony != nil {
		s.state.FinishedBudgetBase = s.state.Ceremony.BudgetBase
	}
	s.state.Ceremony = nil
	s.state.Mode = ModeNormal
}

// RestartTurnBudget gives each accepted ceremony step its own tool-call
// budget; MADE's repeat limits bound the ceremony as a whole. Once the last
// step closed the ceremony, it gives the closing answer a full budget. The
// turn's call count itself is derived from the transcript and never
// rewritten.
func (s *Session) RestartTurnBudget() {
	if s.state.Ceremony != nil {
		s.state.Ceremony.BudgetBase = s.state.TurnCallCount
		return
	}
	s.state.FinishedBudgetBase = s.state.TurnCallCount
}

// NoteStoppedCeremony records that the person stopped the ceremony named by
// definition, so the next prompt can tell the model. On 10 October 2026 the
// model, never told of a /stop-ceremony, called axlr_step_done again on the
// next prompt and asked whether to retry once the ceremony restarted.
func (s *Session) NoteStoppedCeremony(definition string) {
	s.state.StoppedCeremony = definition
}

// TakeStoppedCeremony returns the ceremony the person stopped since the last
// prompt and forgets it, so the model is told once.
func (s *Session) TakeStoppedCeremony() string {
	definition := s.state.StoppedCeremony
	s.state.StoppedCeremony = ""
	return definition
}

// CeremonyChoice is the published definition the person chose with
// /ceremony: its name and version.
type CeremonyChoice struct{ Definition, Version string }

// ChooseCeremony selects a published definition for the next prompt and
// switches the session to ceremony mode.
func (s *Session) ChooseCeremony(definition, version string) error {
	if definition == "" || version == "" {
		return errors.New("a ceremony needs a definition name and a version")
	}
	if err := s.SetMode(ModeCeremony); err != nil {
		return err
	}
	s.state.ChosenCeremony = CeremonyChoice{Definition: definition, Version: version}
	return nil
}

// ChosenCeremony is the definition /ceremony selected, zero when none.
func (s Session) ChosenCeremony() CeremonyChoice { return s.state.ChosenCeremony }
