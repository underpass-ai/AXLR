package domain

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
