package domain

import (
	"errors"
	"slices"
	"time"
)

// ConsoleLine is a line the console adds to a session's transcript for the
// person only. It is anchored after a message, like an archived draft, and
// is never part of the model's history: adding one changes no message, so it
// makes no model request and keeps the prompt cache prefix of later
// requests.
type ConsoleLine struct {
	// AfterMessage is how many messages the session had when the line was
	// added; the line is shown after them.
	AfterMessage int
	At           time.Time
	// Plan is the outcome of a plan the session started.
	Plan *PlanOutcome
}

// PlanOutcome is what a finished plan left, as the session that planned it
// shows it. On 10 October 2026 a plan ran and synced green in about three
// minutes, and the planning session's transcript still ended with the
// planner's "It's waiting for your approval on the plan card".
type PlanOutcome struct {
	PlanID string
	Status PlanStatus
	// Tasks counts the plan's tasks; Done, Blocked and Skipped those that
	// ended so.
	Tasks, Done, Blocked, Skipped int
	// Syncs is each integrated wave's verdict, in order.
	Syncs []SyncVerdict
	// Changed is the files the tasks' hand-backs name, each once.
	Changed []string
	// CostUSD sums the cost the task sessions reported; CostKnown is false
	// when none of them reported one.
	CostUSD   float64
	CostKnown bool
}

// SyncVerdict is one wave's integration verdict.
type SyncVerdict struct {
	Wave    int
	Verdict string
}

// PlanOutcomeReported reports whether a plan's outcome belongs in its
// planning session: it ran to an end on its own. A declined plan already
// tells the model, and an interrupted one resumes.
func PlanOutcomeReported(status PlanStatus) bool {
	return status == PlanDone || status == PlanPartial || status == PlanBlocked
}

// NewPlanOutcome summarises a finished plan from its registry record; the
// cost is the caller's, which knows the task sessions' ledgers.
func NewPlanOutcome(plan PlanRecord) PlanOutcome {
	outcome := PlanOutcome{PlanID: plan.ID, Status: plan.Status, Tasks: len(plan.Tasks)}
	for _, task := range plan.Tasks {
		switch task.Status {
		case TaskDone:
			outcome.Done++
		case TaskBlocked:
			outcome.Blocked++
		case TaskSkipped:
			outcome.Skipped++
		}
		if task.Handback == nil {
			continue
		}
		for _, path := range task.Handback.Changed {
			if path != "" && !slices.Contains(outcome.Changed, path) {
				outcome.Changed = append(outcome.Changed, path)
			}
		}
	}
	for _, sync := range plan.Syncs {
		outcome.Syncs = append(outcome.Syncs, SyncVerdict{Wave: sync.Wave, Verdict: sync.Verdict})
	}
	return outcome
}

func (o PlanOutcome) validate() error {
	if !ValidSlug(o.PlanID) || !PlanOutcomeReported(o.Status) {
		return errors.New("a plan outcome needs the plan's id and a finished status")
	}
	return nil
}

func (o PlanOutcome) clone() PlanOutcome {
	o.Syncs = slices.Clone(o.Syncs)
	o.Changed = slices.Clone(o.Changed)
	return o
}

func (l ConsoleLine) clone() ConsoleLine {
	if l.Plan != nil {
		plan := l.Plan.clone()
		l.Plan = &plan
	}
	return l
}

// ConsoleLines returns a copy of the session's console lines, in order.
func (s Session) ConsoleLines() []ConsoleLine { return s.Export().ConsoleLines }

// AddPlanOutcome adds a finished plan's outcome as a console line after the
// session's last message, between turns. A plan is added once: it reports
// false when the session already shows that plan's outcome.
func (s *Session) AddPlanOutcome(outcome PlanOutcome) (bool, error) {
	if err := s.requireBetweenTurns("console lines"); err != nil {
		return false, err
	}
	if err := outcome.validate(); err != nil {
		return false, err
	}
	for _, line := range s.state.ConsoleLines {
		if line.Plan != nil && line.Plan.PlanID == outcome.PlanID {
			return false, nil
		}
	}
	line := ConsoleLine{AfterMessage: len(s.state.Messages), At: now().UTC().Truncate(time.Second), Plan: &outcome}
	s.state.ConsoleLines = append(s.state.ConsoleLines, line.clone())
	return true, nil
}

// validateConsoleLines checks lines restored with a transcript of messages
// messages: each anchored inside it, in order, and carrying an outcome.
func validateConsoleLines(lines []ConsoleLine, messages int) error {
	previous := 0
	for _, line := range lines {
		if line.AfterMessage < previous || line.AfterMessage > messages || line.Plan == nil {
			return errors.New("console line is not anchored in the transcript")
		}
		if err := line.Plan.validate(); err != nil {
			return err
		}
		previous = line.AfterMessage
	}
	return nil
}
