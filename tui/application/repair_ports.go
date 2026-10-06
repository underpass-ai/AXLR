package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// RepairRegistryPort keeps the durable repair records: which session asked,
// which session repairs, where the clone is and how it ended. It is the link
// between the two sessions and what survives a console that stops.
type RepairRegistryPort interface {
	Load(ctx context.Context) ([]domain.RepairRecord, error)
	// Save adds or replaces the record with the same ID.
	Save(ctx context.Context, record domain.RepairRecord) error
}

// RepairCloneRequest is what the clone needs to know about itself.
type RepairCloneRequest struct {
	Repository, Brief, About, Slug string
	// Issue is the GitHub issue number the brief came from, if any.
	Issue string
	// Origin is the session that asked for the repair; Build the console
	// build that detected the defect. Both go into the clone's marker.
	Origin domain.SessionID
	Build  string
}

// RepairClone is a fresh clone with its marker written.
type RepairClone struct {
	Path, Base string
}

// RepairClonePort clones the repository for one repair and writes the marker
// that lets the repair ceremony start there.
type RepairClonePort interface {
	Prepare(ctx context.Context, request RepairCloneRequest) (RepairClone, error)
}

// RepairWorkbench drives the repair session inside its clone. The console
// builds one per repair, rooted in the clone, so the session's tools and
// checks never leave it.
type RepairWorkbench interface {
	// Begin starts a turn with the prompt; the first one starts the ceremony.
	Begin(ctx context.Context, s *domain.Session, prompt root.Text, emit func(Event) error) error
	// Resolve applies the person's decision to the session's first pending
	// call and carries the turn on.
	Resolve(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) error
	// Decide records the person's merge decision and carries the ceremony on.
	Decide(ctx context.Context, s *domain.Session, approve bool, reason string, emit func(Event) error) error
	// Continue resumes a session the console left interrupted: a console
	// step first, through MADE, then the turn.
	Continue(ctx context.Context, s *domain.Session, emit func(Event) error) error
	// Observe attaches the watcher of the ceremony's progress.
	Observe(observer CeremonyObserverPort)
	Close() error
}

// RepairWorkbenchPort opens a workbench rooted in a clone.
type RepairWorkbenchPort interface {
	Open(ctx context.Context, clone string) (RepairWorkbench, error)
}

// RepairNoticesPort hands a session the messages its repairs left for it.
type RepairNoticesPort interface {
	// Drain returns the notices for the session and marks them delivered.
	Drain(ctx context.Context, session domain.SessionID) ([]string, error)
}

// RepairRequestPort is the host tool side of self-repair.
type RepairRequestPort interface {
	Request(ctx context.Context, s domain.Session, arguments root.JSONValue) (any, error)
	Status(ctx context.Context, s domain.Session, arguments root.JSONValue) (any, error)
}

// RepairEvent tells a watcher that a record changed.
type RepairEvent struct {
	Record domain.RepairRecord
}

// CeremonyObserverPort watches a driver's progress for a host that runs the
// ceremony without a person in front of it.
type CeremonyObserverPort interface {
	Observe(progress CeremonyProgress)
}

// CeremonyProgress is one observed change: a new step, a wait for the person
// or a terminal state with the report the model would have received.
type CeremonyProgress struct {
	Instance, Definition, Step, State string
	Terminal                          bool
	Awaiting                          bool
	Report                            map[string]any
	Repair                            *domain.RepairRun
}
