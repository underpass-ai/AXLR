package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	// DefaultMaxActiveJobs bounds how many repair and improvement sessions
	// run at once, across the consoles that share the registry, when
	// settings.json's jobs.max_active is silent. One at a time made eight
	// /improve and /repair jobs need eight consoles on 9 October 2026.
	DefaultMaxActiveJobs = 2
	// DefaultMaxRepairAttempts bounds how many repair sessions one failure
	// signature may start before the console asks for a hand-made repair.
	DefaultMaxRepairAttempts = 2
	// maxRepairNudges bounds the console turns that remind the repair model
	// of a step it left open; maxRepairRetries the resumptions after a model
	// stream failed.
	maxRepairNudges  = 3
	maxRepairRetries = 2
	// decisionHandoff bounds how long the person's decision waits for the
	// repair loop to take it.
	decisionHandoff = 2 * time.Second
)

// RepairSettings is what settings.json decides about self-repair, as the
// coordinator needs it.
type RepairSettings struct {
	// Repository is owner/name; About the KMP project about; Directory the
	// absolute directory the clones live in.
	Repository, About, Directory string
	// MaxAttempts bounds repair sessions per failure signature; zero means
	// DefaultMaxRepairAttempts.
	MaxAttempts int
	// MaxActive bounds the repair and improvement sessions active in the
	// registry, whoever started them; zero means DefaultMaxActiveJobs.
	MaxActive int
	// Watch bounds the merge queue's wait for one job's checks
	// (repair.watch_minutes); zero means the ceremony's default.
	Watch time.Duration
}

// SelfRepair lets the agent of a running session ask the console to repair
// AXLR itself. The console validates the request against the session's own
// transcript, refuses what belongs to the project or to an external service,
// clones the repository, starts a separate repair session rooted in the clone
// and drives it without a person in front of it: the reproduction command
// and the merge keep their approvals, surfaced to the person through the
// records. The original session continues and is told the outcome.
type SelfRepair struct {
	Registry  RepairRegistryPort
	Clones    RepairClonePort
	Workbench RepairWorkbenchPort
	Store     SessionStorePort
	// Engine is MADE as the parent console sees it; nil means not connected.
	Engine   CeremonyEnginePort
	Settings RepairSettings
	// Forge reads the jobs' pull requests for /jobs and its merge queue;
	// nil means neither is available.
	Forge ForgePort
	// Sleep spaces the merge queue's reads; nil means a timer.
	Sleep func(context.Context, time.Duration) error
	// Build is the console build that detects defects and receives notices.
	Build string
	// Candidates builds the repaired axlr-tui from a clone once its checks
	// are green and installs it on request; nil builds nothing.
	Candidates RepairCandidatePort
	// AutoInstall installs a merged repair's candidate without the panel's
	// i (settings' repair.install).
	AutoInstall bool
	// RunToken names this console launch; a record left running under
	// another token belongs to a console that stopped.
	RunToken string
	// Lifetime bounds the background repair sessions; nil means Background.
	Lifetime context.Context
	Now      func() time.Time
	NewID    func() (domain.SessionID, error)

	mu     sync.Mutex
	runs   map[string]*repairRun
	events chan RepairEvent
	closed bool
	wg     sync.WaitGroup
	// unsaved is the record changes the registry refused since Records last
	// reported them: the registry cannot show those itself.
	unsaved error
	// draining is true while the merge queue's goroutine runs; queueCancel
	// stops it with the console.
	draining    bool
	queueCtx    context.Context
	queueCancel context.CancelFunc
}

// repairRun is one repair session the console drives in the background.
type repairRun struct {
	cancel    context.CancelFunc
	decisions chan repairDecision
	// record, waiting and progress are guarded by SelfRepair.mu.
	record   domain.RepairRecord
	waiting  bool
	progress CeremonyProgress
}

type repairDecision struct {
	approve bool
	reason  string
}

var _ RepairRequestPort = (*SelfRepair)(nil)
var _ RepairNoticesPort = (*SelfRepair)(nil)

func (r *SelfRepair) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *SelfRepair) lifetime() context.Context {
	if r.Lifetime != nil {
		return r.Lifetime
	}
	return context.Background()
}

func (r *SelfRepair) newID() (domain.SessionID, error) {
	if r.NewID != nil {
		return r.NewID()
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return domain.NewSessionID(hex.EncodeToString(random[:]))
}

func (r *SelfRepair) maxAttempts() int {
	if r.Settings.MaxAttempts > 0 {
		return r.Settings.MaxAttempts
	}
	return DefaultMaxRepairAttempts
}

func (r *SelfRepair) maxActive() int {
	if r.Settings.MaxActive > 0 {
		return r.Settings.MaxActive
	}
	return DefaultMaxActiveJobs
}

func (r *SelfRepair) configured() error {
	switch {
	case r == nil || r.Registry == nil || r.Clones == nil || r.Workbench == nil || r.Store == nil:
		return errors.New("self-repair is not configured in this console")
	case r.Engine == nil:
		return errors.New("MADE is not connected; self-repair needs the axlr_repair ceremony")
	case r.Settings.Repository == "":
		return errors.New("settings repair.repository names no repository")
	}
	return nil
}

// Events delivers one event per record change, for a host that shows the
// repairs. Events are dropped, never blocked on, when nobody reads them; the
// registry holds the truth.
func (r *SelfRepair) Events() <-chan RepairEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events == nil {
		r.events = make(chan RepairEvent, 64)
	}
	return r.events
}

func (r *SelfRepair) publish(record domain.RepairRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events == nil || r.closed {
		return
	}
	select {
	case r.events <- RepairEvent{Record: record}:
	default:
	}
}

// save persists the record and tells the watchers. The write outlives a
// cancelled caller: a record that says running after the console stopped
// would hide an interrupted repair. A change the registry refuses is not
// published as if it were saved: it is kept for Records to report, and the
// watchers are told to reload what the registry holds, so they do.
func (r *SelfRepair) save(ctx context.Context, record *domain.RepairRecord) error {
	record.Updated = r.now()
	ctx = context.WithoutCancel(ctx)
	if err := r.Registry.Save(ctx, *record); err != nil {
		err = fmt.Errorf("repair %s: the registry did not save its latest change: %w", record.ID, err)
		r.mu.Lock()
		r.unsaved = errors.Join(r.unsaved, err)
		r.mu.Unlock()
		if stored, loadErr := r.Registry.Load(ctx); loadErr == nil {
			for _, candidate := range stored {
				if candidate.ID == record.ID {
					r.publish(candidate)
				}
			}
		}
		return err
	}
	r.publish(*record)
	return nil
}

// Request is the axlr_request_repair host tool: validate, refuse what is not
// a defect of AXLR, refuse duplicates and exhausted attempts, clone, create
// the repair session and start driving it.
func (r *SelfRepair) Request(ctx context.Context, s domain.Session, arguments root.JSONValue) (any, error) {
	if err := r.configured(); err != nil {
		return nil, err
	}
	request, err := decodeRepairRequest(arguments)
	if err != nil {
		return nil, err
	}
	state := s.Export()
	if s.Mode() == domain.ModeRepair {
		return nil, errors.New("a repair session cannot request another repair; finish this one and report the defect to the user")
	}
	if s.Mode() == domain.ModeImprove {
		return nil, errors.New("an improvement session cannot request a repair; finish this one and report the defect to the user")
	}
	if dir := r.Settings.Directory; dir != "" && strings.HasPrefix(string(state.Workspace)+string(filepath.Separator), filepath.Clean(dir)+string(filepath.Separator)) {
		return nil, errors.New("this workspace is a repair clone; a repair cannot start another repair from it")
	}
	if cause := externalCause(request.texts()...); cause != "" {
		return nil, fmt.Errorf("the request names an external cause (%q): a provider, credential, permission or network problem is not a defect of AXLR; resolve it or tell the user", cause)
	}
	failures, refusal := citedFailures(s, request)
	if refusal != "" {
		return nil, errors.New(refusal)
	}
	if !recurs(s, failures) {
		return nil, errors.New("an isolated failure is not evidence of a defect: repeat the operation once; if it fails the same way, cite both calls (a wrong result needs two cited calls)")
	}
	signature := repairSignature(r.Settings.Repository, failures)
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the repair registry: %w", err)
	}
	if refusal := r.admit(records, state.ID, signature); refusal != "" {
		return nil, errors.New(refusal)
	}
	if err := r.Engine.Ready(ctx, "axlr_repair", "1.0"); err != nil {
		return nil, fmt.Errorf("MADE cannot run the repair ceremony: %w", err)
	}
	attempts := 0
	for _, record := range records {
		if record.Signature == signature && record.Session != "" {
			attempts++
		}
	}
	brief := repairBrief(s, request, failures, r.Build)
	record := domain.RepairRecord{Signature: signature, Brief: brief, Attempt: attempts + 1}
	if err := r.start(ctx, state, records, &record, request.Description); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true, "repair": record.ID, "session": record.Session, "clone": record.Clone, "repository": record.Repository, "status": record.Status, "attempt": record.Attempt,
		"instruction": "The repair runs in a separate session rooted in the clone; this session continues its task. The reproduction command and the merge wait for the person on the /repair panel. Consult axlr_repair_status for progress; do not request the same repair again. A merged repair does not change this running console: it needs an updated build and a restart."}, nil
}

// RequestImprovement is the axlr_request_improvement host tool: validate the
// cited calls, apply the improvement admission rules, clone and start an
// improvement session the console drives like a repair session.
func (r *SelfRepair) RequestImprovement(ctx context.Context, s domain.Session, arguments root.JSONValue) (any, error) {
	if err := r.configured(); err != nil {
		return nil, err
	}
	request, err := decodeImprovementRequest(arguments)
	if err != nil {
		return nil, err
	}
	state := s.Export()
	switch s.Mode() {
	case domain.ModeRepair:
		return nil, errors.New("a repair session cannot request an improvement; finish this one and tell the user what you would improve")
	case domain.ModeImprove:
		return nil, errors.New("an improvement session cannot request another improvement; finish this one and tell the user what you would improve")
	case domain.ModeTask, domain.ModePlan:
		return nil, errors.New("plan and task sessions cannot request an improvement; leave a note for the person instead")
	}
	if dir := r.Settings.Directory; dir != "" && strings.HasPrefix(string(state.Workspace)+string(filepath.Separator), filepath.Clean(dir)+string(filepath.Separator)) {
		return nil, errors.New("this workspace is a repair or improvement clone; an improvement cannot start from it")
	}
	if cause := externalCause(request.texts()...); cause != "" {
		return nil, fmt.Errorf("the request names an external cause (%q): a provider, credential, permission or network problem is not something AXLR can improve; resolve it or tell the user", cause)
	}
	calls, refusal := citedFriction(s, request)
	if refusal != "" {
		return nil, errors.New(refusal)
	}
	signature := improvementSignature(r.Settings.Repository, calls, request.Description)
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the repair registry: %w", err)
	}
	if refusal := r.admitImprovement(records, state.ID, signature); refusal != "" {
		return nil, errors.New(refusal)
	}
	if err := r.Engine.Ready(ctx, "axlr_improve", "1.0"); err != nil {
		return nil, fmt.Errorf("MADE cannot run the improve ceremony: %w", err)
	}
	record := domain.RepairRecord{Improvement: true, Signature: signature, Brief: improvementBrief(s, request, calls, r.Build), Attempt: 1}
	if err := r.start(ctx, state, records, &record, request.Description); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true, "improvement": record.ID, "session": record.Session, "clone": record.Clone, "repository": record.Repository, "status": record.Status,
		"instruction": "The improvement runs in a separate session rooted in the clone; this session continues its task. Its check command and the merge wait for the person on the /improve panel, and the merge is always the person's. Consult axlr_repair_status for progress; do not request it again. A merged improvement does not change this running console: it needs an updated build and a restart."}, nil
}

// start records a repair or improvement, prepares its clone, creates the
// session that works there in the matching mode and drives it in the
// background. The record carries Improvement, Signature, Brief and Attempt;
// state is the session that asked, whose model the new session uses.
func (r *SelfRepair) start(ctx context.Context, state domain.SessionState, records []domain.RepairRecord, record *domain.RepairRecord, description string) error {
	now := r.now()
	kind, mode, cloneKind := "repair", domain.ModeRepair, ""
	if record.Improvement {
		kind, mode, cloneKind = "improvement", domain.ModeImprove, ImproveKind
	}
	record.ID, record.Repository, record.Parent, record.Build = RepairSlug(description, now.UTC()), r.Settings.Repository, state.ID, r.Build
	record.Status, record.RunToken, record.Created = domain.RepairPreparing, r.RunToken, now
	for _, existing := range records {
		if existing.ID == record.ID {
			record.ID += "-" + record.Signature[:4]
		}
	}
	if err := r.save(ctx, record); err != nil {
		return fmt.Errorf("record the %s: %w", kind, err)
	}
	clone, err := r.Clones.Prepare(ctx, RepairCloneRequest{Repository: r.Settings.Repository, Brief: record.Brief, About: r.about(), Slug: record.ID, Kind: cloneKind, Origin: state.ID, Build: r.Build})
	if err != nil {
		record.Status, record.Error = domain.RepairFailed, bounded("clone: "+err.Error(), 600)
		record.Notice = repairNotice(*record, r.Build)
		return errors.Join(fmt.Errorf("the clone could not be prepared, so no %s session started: %w", kind, err), r.save(ctx, record))
	}
	record.Clone = clone.Path
	childID, err := r.newID()
	if err != nil {
		return r.fail(ctx, record, err)
	}
	child, err := domain.NewSession(childID, domain.Workspace(clone.Path), state.Model)
	if err != nil {
		return r.fail(ctx, record, err)
	}
	if err := child.SetMode(mode); err != nil {
		return r.fail(ctx, record, err)
	}
	if err := r.Store.Save(ctx, child); err != nil {
		return r.fail(ctx, record, fmt.Errorf("save the %s session: %w", kind, err))
	}
	record.Session = childID
	workbench, err := r.Workbench.Open(ctx, clone.Path)
	if err != nil {
		return r.fail(ctx, record, fmt.Errorf("open the %s workbench: %w", kind, err))
	}
	record.Status = domain.RepairRunning
	if err := r.save(ctx, record); err != nil {
		_ = workbench.Close()
		return fmt.Errorf("record the %s: %w", kind, err)
	}
	if err := r.launch(*record, child, workbench, root.Text(record.Brief), true); err != nil {
		return r.fail(ctx, record, err)
	}
	return nil
}

func (r *SelfRepair) about() string {
	if r.Settings.About != "" {
		return r.Settings.About
	}
	return "project:" + strings.ToLower(r.Settings.Repository[strings.LastIndex(r.Settings.Repository, "/")+1:])
}

func (r *SelfRepair) fail(ctx context.Context, record *domain.RepairRecord, err error) error {
	record.Status, record.Error = domain.RepairFailed, bounded(err.Error(), 600)
	record.Notice = repairNotice(*record, r.Build)
	return errors.Join(err, r.save(ctx, record))
}

// admit applies the duplicate, recursion, concurrency and attempt rules.
func (r *SelfRepair) admit(records []domain.RepairRecord, session domain.SessionID, signature string) string {
	active := 0
	attempts := 0
	for _, record := range records {
		if record.Session == session {
			return fmt.Sprintf("this session is the repair session of %s; a repair cannot request another repair", record.ID)
		}
		if record.Active() {
			active++
			if record.Signature == signature {
				return fmt.Sprintf("duplicate: repair %s is already %s for this failure; consult axlr_repair_status instead of requesting it again", record.ID, record.Status)
			}
		}
		if record.Signature != signature {
			continue
		}
		switch record.Status {
		case domain.RepairInterrupted:
			return fmt.Sprintf("repair %s for this failure was interrupted; the person recovers it from the /repair panel instead of starting another", record.ID)
		case domain.RepairCompleted:
			if record.Build == r.Build && record.Installed != "" {
				return fmt.Sprintf("already repaired: pull request #%d (%s) merged this failure's fix, and build %s with it is installed at %s, but this console still runs build %s; the person restarts the console instead of repairing again", record.PullRequest, record.URL, record.CandidateVersion, record.Installed, r.Build)
			}
			if record.Build == r.Build {
				return fmt.Sprintf("already repaired: pull request #%d (%s) merged this failure's fix as %s, but this console still runs build %s; update or rebuild AXLR and restart instead of repairing again", record.PullRequest, record.URL, record.MergeSHA, r.Build)
			}
		}
		if record.Session != "" {
			attempts++
		}
	}
	if limit := r.maxActive(); active >= limit {
		return fmt.Sprintf("another repair is active (%d of %d allowed by jobs.max_active); wait for it or consult axlr_repair_status", active, limit)
	}
	if attempts >= r.maxAttempts() {
		return fmt.Sprintf("attempts exhausted: %d repair sessions already ran for this failure; repair it by hand with axlr-tui --repair \"<brief>\"", attempts)
	}
	return ""
}

// launch starts driving the repair session in the background.
func (r *SelfRepair) launch(record domain.RepairRecord, child domain.Session, workbench RepairWorkbench, prompt root.Text, fresh bool) error {
	ctx, cancel := context.WithCancel(r.lifetime())
	run := &repairRun{cancel: cancel, decisions: make(chan repairDecision), record: record}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return errors.New("the console is closing; the repair was not started")
	}
	if r.runs == nil {
		r.runs = map[string]*repairRun{}
	}
	r.runs[record.ID] = run
	r.wg.Add(1)
	r.mu.Unlock()
	workbench.Observe(runObserver{repair: r, run: run})
	go func() {
		defer r.wg.Done()
		defer cancel()
		r.drive(ctx, run, child, workbench, prompt, fresh)
	}()
	return nil
}

// update changes the run's record under the lock and persists it.
func (r *SelfRepair) update(ctx context.Context, run *repairRun, change func(*domain.RepairRecord)) domain.RepairRecord {
	r.mu.Lock()
	change(&run.record)
	record := run.record
	r.mu.Unlock()
	_ = r.save(ctx, &record) // a refusal is kept for Records to report
	r.mu.Lock()
	run.record.Updated = record.Updated
	r.mu.Unlock()
	return record
}

func (r *SelfRepair) last(run *repairRun) CeremonyProgress {
	r.mu.Lock()
	defer r.mu.Unlock()
	return run.progress
}

// drive is the repair session's loop: it runs turns, surfaces the two
// decisions the person keeps, nudges a model that leaves a step open, and
// ends with the ceremony's outcome, an interruption or a failure.
func (r *SelfRepair) drive(ctx context.Context, run *repairRun, child domain.Session, workbench RepairWorkbench, prompt root.Text, fresh bool) {
	defer workbench.Close()
	defer r.forget(run)
	var err error
	if fresh {
		err = workbench.Begin(ctx, &child, prompt, nil)
	} else {
		err = workbench.Continue(ctx, &child, nil)
	}
	nudges, retries := 0, 0
	for {
		r.update(ctx, run, func(record *domain.RepairRecord) { syncRecord(record, child) })
		if ctx.Err() != nil {
			r.update(ctx, run, func(record *domain.RepairRecord) {
				record.Status, record.Pending = domain.RepairInterrupted, ""
				record.Error = "interrupted while " + describeRunState(child)
				record.Notice = repairNotice(*record, r.Build)
			})
			return
		}
		if err != nil {
			retries++
			if retries > maxRepairRetries {
				r.finish(ctx, run, domain.RepairFailed, bounded(err.Error(), 600))
				return
			}
			r.update(ctx, run, func(record *domain.RepairRecord) { record.Error = bounded(err.Error(), 600) })
			err = workbench.Continue(ctx, &child, nil)
			continue
		}
		if progress := r.last(run); progress.Terminal {
			r.conclude(ctx, run, progress)
			if progress.State == "COMPLETED" {
				// A merge that did not wait for the person (auto_merge)
				// builds its candidate now.
				r.buildCandidate(ctx, run)
				r.installMerged(ctx, run)
				r.update(ctx, run, func(record *domain.RepairRecord) { record.Notice = repairNotice(*record, r.Build) })
			}
			return
		}
		ceremony, live := child.Ceremony()
		switch {
		case !live:
			r.finish(ctx, run, domain.RepairFailed, "the repair session ended without a ceremony outcome; its transcript is kept")
			return
		case ceremony.AwaitingPerson():
			pending := "the person's decision on the approval card"
			if ceremony.Repair != nil {
				if reason := r.buildCandidate(ctx, run); reason != "" && CanDecline(ceremony) {
					// A console that does not build is declined as a red
					// check would be: the pull request stays open.
					decideErr := workbench.Decide(ctx, &child, false, reason, nil)
					if decideErr == nil {
						continue
					}
					// The card stays for the person, with the reason.
					r.update(ctx, run, func(record *domain.RepairRecord) {
						record.Error = bounded(reason+"; declining it failed: "+decideErr.Error(), 600)
					})
				}
				pending = r.mergePending(ctx, run, ceremony.Repair.PullRequest, ceremony.Repair.URL)
			}
			decision, ok := r.await(ctx, run, domain.RepairAwaitingMerge, pending)
			if !ok {
				continue
			}
			if decideErr := workbench.Decide(ctx, &child, decision.approve, decision.reason, nil); decideErr != nil {
				r.update(ctx, run, func(record *domain.RepairRecord) { record.Error = bounded("merge decision: "+decideErr.Error(), 600) })
			}
		case child.Status() == domain.StatusApproval && len(child.Pending()) > 0:
			call := child.Pending()[0]
			decision, ok := r.await(ctx, run, domain.RepairAwaitingApproval, describePending(call))
			if !ok {
				continue
			}
			verdict := domain.DecisionDeny
			if decision.approve {
				verdict = domain.DecisionApprove
			}
			if resolveErr := workbench.Resolve(ctx, &child, call.Call.ID, verdict, nil); resolveErr != nil {
				err = resolveErr
			}
		case child.Status() == domain.StatusComplete:
			nudges++
			if nudges > maxRepairNudges {
				r.finish(ctx, run, domain.RepairFailed, fmt.Sprintf("the model ended its turn %d times with step %s open", nudges-1, ceremony.Step))
				return
			}
			err = workbench.Begin(ctx, &child, repairNudge(ceremony, nudges), nil)
		default:
			retries++
			if retries > maxRepairRetries {
				r.finish(ctx, run, domain.RepairFailed, "the repair session stalled in state "+string(child.Status()))
				return
			}
			err = workbench.Continue(ctx, &child, nil)
		}
	}
}

// await parks the run until the person decides on the record, or the
// console stops.
func (r *SelfRepair) await(ctx context.Context, run *repairRun, status domain.RepairStatus, pending string) (repairDecision, bool) {
	r.update(ctx, run, func(record *domain.RepairRecord) { record.Status, record.Pending = status, pending })
	r.mu.Lock()
	run.waiting = true
	queued := status == domain.RepairAwaitingMerge && !run.record.Queued.IsZero()
	r.mu.Unlock()
	if queued {
		// The person queued this merge: the queue takes it when its turn
		// comes and its checks are green on an up-to-date branch.
		r.kickQueue()
	}
	defer func() {
		r.mu.Lock()
		run.waiting = false
		r.mu.Unlock()
	}()
	select {
	case decision := <-run.decisions:
		r.update(ctx, run, func(record *domain.RepairRecord) { record.Status, record.Pending = domain.RepairRunning, "" })
		return decision, true
	case <-ctx.Done():
		return repairDecision{}, false
	}
}

func (r *SelfRepair) finish(ctx context.Context, run *repairRun, status domain.RepairStatus, reason string) {
	r.update(ctx, run, func(record *domain.RepairRecord) {
		record.Status, record.Pending, record.Error = status, "", reason
		record.Notice = repairNotice(*record, r.Build)
	})
}

// conclude records the ceremony's terminal state: the merged pull request,
// or why it stopped, plus what KMP said about the cause and the outcome.
func (r *SelfRepair) conclude(ctx context.Context, run *repairRun, progress CeremonyProgress) {
	r.update(ctx, run, func(record *domain.RepairRecord) {
		record.State, record.Step, record.Pending = progress.State, progress.Step, ""
		if progress.Instance != "" {
			record.Instance = progress.Instance
		}
		if repair := progress.Repair; repair != nil {
			if repair.PullRequest > 0 {
				record.PullRequest, record.URL = repair.PullRequest, repair.URL
			}
			if repair.MergeSHA != "" {
				record.MergeSHA = repair.MergeSHA
			}
		}
		if memory, _ := progress.Report["memory"].(string); memory != "" {
			record.Memory = memory
		}
		if check := checkSummary(progress.Report); check != "" {
			record.Check = check
		}
		if sha, _ := progress.Report["merge_sha"].(string); sha != "" {
			record.MergeSHA = sha
		}
		switch progress.State {
		case "COMPLETED":
			record.Status, record.Error = domain.RepairCompleted, ""
		default:
			record.Status = domain.RepairBlocked
			record.Error = bounded(blockedReason(progress.Report), 600)
		}
		record.Notice = repairNotice(*record, r.Build)
	})
}

func blockedReason(report map[string]any) string {
	if reason, _ := report["reason"].(string); reason != "" {
		return reason
	}
	if watch, ok := report["watch"].(map[string]any); ok {
		if reason, _ := watch["reason"].(string); reason != "" {
			return reason
		}
	}
	for _, key := range []string{"error", "feedback", "reconciled"} {
		if text, _ := report[key].(string); text != "" {
			return text
		}
	}
	if check, ok := report["check"].(map[string]any); ok {
		if tail, _ := check["output_tail"].(string); tail != "" {
			return "the ceremony ended BLOCKED; last check output: " + tail
		}
	}
	return "the ceremony ended BLOCKED"
}

func (r *SelfRepair) forget(run *repairRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, candidate := range r.runs {
		if candidate == run {
			delete(r.runs, id)
		}
	}
}

// syncRecord mirrors the repair session's ceremony into the record.
func syncRecord(record *domain.RepairRecord, child domain.Session) {
	ceremony, live := child.Ceremony()
	if !live {
		return
	}
	record.Instance, record.Step = ceremony.Instance, ceremony.Step
	record.StepAttempt, record.StepLimit = ceremony.Iteration, AttemptLimit(ceremony)
	if repair := ceremony.Repair; repair != nil && repair.PullRequest > 0 {
		record.PullRequest, record.URL = repair.PullRequest, repair.URL
	}
}

func describeRunState(child domain.Session) string {
	ceremony, live := child.Ceremony()
	if !live {
		return "the session had no ceremony"
	}
	return fmt.Sprintf("step %s of %s (%s)", ceremony.Step, ceremony.Definition, child.Status())
}

// describePending names the decision the person owes: the reproduction
// command, or a tool the policy keeps under approval.
func describePending(call domain.PendingTool) string {
	if call.Call.Name == HostStepDoneName {
		if done, err := decodeStepDone(call.Call.Arguments); err == nil {
			if command, ok := done.command(); ok {
				return fmt.Sprintf("run the check command %s %s in the clone: a approves it once for the ceremony, d denies it", command.Program, strings.Join(command.Args, " "))
			}
		}
	}
	return fmt.Sprintf("approve %s %s: a runs it, d denies it", call.Call.Name, bounded(singleLineText(string(call.Call.Arguments.Bytes())), 300))
}

func repairNudge(run domain.CeremonyRun, nudge int) root.Text {
	kind, escape := "repair", "reproducible=false in reproduce"
	if improving(run) {
		kind, escape = "improvement", "feasible=false in brief"
	}
	return root.Text(fmt.Sprintf("[AXLR] Nobody is reading this %s session: the %s step of %s is still open (reminder %d of %d). Do the step's work and hand it back with axlr_step_done; if it cannot be done, say so through axlr_step_done (%s) instead of ending your turn. %s", kind, run.Step, run.Definition, nudge, maxRepairNudges, escape, baseInstruction(run)))
}

// repairNotice is the message the parent session receives when the repair
// ends. It never claims the running console is fixed: a merge changes the
// repository, not the process that detected the defect.
func repairNotice(record domain.RepairRecord, build string) string {
	var text strings.Builder
	label, change, panel := "Self-repair", "the fix", "/repair"
	if record.Improvement {
		label, change, panel = "Self-improvement", "the change", "/improve"
	}
	switch record.Status {
	case domain.RepairCompleted:
		fmt.Fprintf(&text, "[AXLR] %s %s merged pull request #%d (%s) into %s as %s.", label, record.ID, record.PullRequest, record.URL, record.Repository, record.MergeSHA)
		switch {
		case record.Installed != "":
			fmt.Fprintf(&text, " Build %s of %s is installed at %s (the previous one is kept as %s); this console still runs build %s until it restarts: restart it before relying on %s.", record.CandidateVersion, change, record.Installed, record.Backup, build, change)
		case record.Candidate != "":
			fmt.Fprintf(&text, " This console still runs build %s, which does not contain %s. The repaired console is built at %s (%s): install it with i on the %s panel, or run it to try, then restart.", build, change, record.Candidate, record.CandidateVersion, panel)
		default:
			fmt.Fprintf(&text, " This console still runs build %s, which does not contain %s: update or rebuild AXLR and restart the console before relying on it.", build, change)
		}
		if record.Error != "" {
			fmt.Fprintf(&text, " %s.", strings.TrimSuffix(record.Error, "."))
		}
	case domain.RepairBlocked:
		fmt.Fprintf(&text, "[AXLR] %s %s ended BLOCKED at step %s: %s.", label, record.ID, record.Step, record.Error)
		if record.URL != "" {
			fmt.Fprintf(&text, " Pull request #%d (%s) is left as it is.", record.PullRequest, record.URL)
		}
		fmt.Fprintf(&text, " The clone %s and session %s keep the evidence (axlr-tui --root <clone> --session <session>).", record.Clone, record.Session)
	case domain.RepairFailed:
		fmt.Fprintf(&text, "[AXLR] %s %s failed: %s.", label, record.ID, record.Error)
		if record.Clone != "" {
			fmt.Fprintf(&text, " The clone %s keeps the evidence", record.Clone)
			if record.Session != "" {
				fmt.Fprintf(&text, " with session %s", record.Session)
			}
			text.WriteString(".")
		}
	case domain.RepairInterrupted:
		fmt.Fprintf(&text, "[AXLR] %s %s was interrupted (%s); the person can recover it from the %s panel without repeating what already happened.", label, record.ID, record.Error, panel)
	default:
		return ""
	}
	if record.Instance != "" {
		fmt.Fprintf(&text, " MADE instance %s.", record.Instance)
	}
	if record.Memory != "" {
		fmt.Fprintf(&text, " KMP: %s.", record.Memory)
	} else if record.Status.Terminal() {
		text.WriteString(" KMP: the ceremony reported no memory write.")
	}
	return text.String()
}

// runObserver mirrors the driver's progress into the record while a console
// step runs for minutes, so the panel shows propose, watch and decide.
type runObserver struct {
	repair *SelfRepair
	run    *repairRun
}

func (o runObserver) Observe(progress CeremonyProgress) {
	o.repair.mu.Lock()
	o.run.progress = progress
	o.repair.mu.Unlock()
	o.repair.update(context.Background(), o.run, func(record *domain.RepairRecord) {
		if progress.Instance != "" {
			record.Instance = progress.Instance
		}
		if progress.Step != "" {
			record.Step, record.StepAttempt, record.StepLimit = progress.Step, progress.StepAttempt, progress.StepLimit
		}
		if check := checkSummary(progress.Report); check != "" {
			record.Check = check
		}
		if progress.State != "" {
			record.State = progress.State
		}
		if repair := progress.Repair; repair != nil && repair.PullRequest > 0 {
			record.PullRequest, record.URL = repair.PullRequest, repair.URL
		}
		if memory, _ := progress.Report["memory"].(string); memory != "" {
			record.Memory = memory
		}
	})
}

// Decide hands the person's decision to a waiting repair: the reproduction
// command or the merge.
func (r *SelfRepair) Decide(ctx context.Context, id string, approve bool, reason string) error {
	r.mu.Lock()
	run, ok := r.runs[id]
	waiting := ok && run.waiting
	status := domain.RepairStatus("")
	if ok {
		status = run.record.Status
	}
	r.mu.Unlock()
	switch {
	case !ok:
		return fmt.Errorf("repair %s is not running in this console", id)
	case !waiting:
		return fmt.Errorf("repair %s is not waiting for a decision", id)
	case status == domain.RepairAwaitingMerge && !approve && strings.TrimSpace(reason) == "":
		return errors.New("say why the merge is declined")
	}
	select {
	case run.decisions <- repairDecision{approve: approve, reason: strings.TrimSpace(reason)}:
		// The run took it: it no longer waits, even before its record
		// says so, which the merge queue relies on.
		r.mu.Lock()
		run.waiting = false
		r.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(decisionHandoff):
		return fmt.Errorf("repair %s did not take the decision; try again", id)
	}
}

// Recover reattaches an interrupted repair: the saved session continues from
// what MADE and the forge already recorded, so nothing is proposed or merged
// twice.
func (r *SelfRepair) Recover(ctx context.Context, id string) error {
	if err := r.configured(); err != nil {
		return err
	}
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return err
	}
	var record domain.RepairRecord
	found := false
	active := 0
	for _, candidate := range records {
		if candidate.ID == id {
			record, found = candidate, true
		}
		if candidate.Active() {
			active++
		}
	}
	switch {
	case !found:
		return fmt.Errorf("repair %s is unknown", id)
	case record.Status != domain.RepairInterrupted:
		return fmt.Errorf("repair %s is %s; only an interrupted repair is recovered", id, record.Status)
	case record.Session == "" || record.Clone == "":
		return fmt.Errorf("repair %s has no session to recover; it stopped before the clone was ready", id)
	case active >= r.maxActive():
		return fmt.Errorf("another repair is active (%d of %d allowed by jobs.max_active)", active, r.maxActive())
	}
	r.mu.Lock()
	_, running := r.runs[id]
	r.mu.Unlock()
	if running {
		return fmt.Errorf("repair %s is already running", id)
	}
	child, err := r.Store.Load(ctx, record.Session)
	if err != nil {
		return fmt.Errorf("load the repair session: %w", err)
	}
	workbench, err := r.Workbench.Open(ctx, record.Clone)
	if err != nil {
		return fmt.Errorf("open the clone %s: %w", record.Clone, err)
	}
	record.Status, record.RunToken, record.Error, record.Pending, record.Notice, record.Notified = domain.RepairRunning, r.RunToken, "", "", "", false
	// A merge queued before the console stopped is queued again by the
	// person, who sees the job anew.
	record.Queued, record.QueueNote = time.Time{}, ""
	if err := r.save(ctx, &record); err != nil {
		_ = workbench.Close()
		return err
	}
	if err := r.launch(record, child, workbench, "", false); err != nil {
		_ = workbench.Close()
		return err
	}
	return nil
}

// Reconcile marks the repairs a stopped console left running as interrupted,
// so the person sees them and can recover them. Called at launch.
func (r *SelfRepair) Reconcile(ctx context.Context) error {
	if r == nil || r.Registry == nil {
		return nil
	}
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !record.Active() || record.RunToken == r.RunToken {
			continue
		}
		record.Status, record.Pending = domain.RepairInterrupted, ""
		record.Error = "the console that drove it stopped"
		record.Notice = repairNotice(record, r.Build)
		if err := r.save(ctx, &record); err != nil {
			return err
		}
	}
	return nil
}

// Records lists every repair, newest first. It also reports, once, the record
// changes the registry refused since the last call, so the panel shows them.
func (r *SelfRepair) Records(ctx context.Context) ([]domain.RepairRecord, error) {
	if r == nil || r.Registry == nil {
		return nil, nil
	}
	r.mu.Lock()
	unsaved := r.unsaved
	r.unsaved = nil
	r.mu.Unlock()
	records, err := r.records(ctx)
	if err != nil {
		return nil, errors.Join(err, unsaved)
	}
	return records, unsaved
}

func (r *SelfRepair) records(ctx context.Context) ([]domain.RepairRecord, error) {
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Created.After(records[j].Created) })
	return records, nil
}

// Drain hands the parent session the notices of its repairs that ended, once.
func (r *SelfRepair) Drain(ctx context.Context, session domain.SessionID) ([]string, error) {
	if r == nil || r.Registry == nil {
		return nil, nil
	}
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	var notices []string
	for _, record := range records {
		if record.Parent != session || record.Notice == "" || record.Notified {
			continue
		}
		record.Notified = true
		if err := r.save(ctx, &record); err != nil {
			return notices, err
		}
		notices = append(notices, record.Notice)
	}
	return notices, nil
}

// Status is the axlr_repair_status host tool: the repairs this session asked
// for, or one repair by ID.
func (r *SelfRepair) Status(ctx context.Context, s domain.Session, arguments root.JSONValue) (any, error) {
	if r == nil || r.Registry == nil {
		return nil, errors.New("self-repair is not configured in this console")
	}
	args, err := decodeHostArguments(arguments, "repair")
	if err != nil {
		return nil, err
	}
	wanted := ""
	if raw, ok := args["repair"]; ok {
		if err := json.Unmarshal(raw, &wanted); err != nil || wanted == "" {
			return nil, errors.New("repair must be a nonempty repair ID")
		}
	}
	records, err := r.records(ctx)
	if err != nil {
		return nil, err
	}
	state := s.Export()
	var out []map[string]any
	for _, record := range records {
		if wanted != "" && record.ID != wanted || wanted == "" && record.Parent != state.ID && record.Session != state.ID {
			continue
		}
		entry := map[string]any{"repair": record.ID, "kind": record.Kind(), "status": record.Status, "repository": record.Repository, "attempt": record.Attempt, "updated": record.Updated.UTC().Format(time.RFC3339)}
		for key, value := range map[string]string{"step": record.Step, "state": record.State, "instance": record.Instance, "url": record.URL, "merge_sha": record.MergeSHA, "pending": record.Pending, "error": record.Error, "memory": record.Memory, "clone": record.Clone, "session": string(record.Session), "parent": string(record.Parent), "candidate": record.Candidate, "candidate_version": record.CandidateVersion, "installed": record.Installed, "backup": record.Backup} {
			if value != "" {
				entry[key] = value
			}
		}
		if record.PullRequest > 0 {
			entry["pull_request"] = record.PullRequest
		}
		out = append(out, entry)
		if len(out) == 20 {
			break
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	return map[string]any{"repairs": out, "build": r.Build, "note": "A completed repair or improvement changed the repository, not this running console; it needs an updated build and a restart. Decisions the person owes appear in pending."}, nil
}

// Close stops the background repairs; each marks itself interrupted.
func (r *SelfRepair) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.closed = true
	if r.queueCancel != nil {
		r.queueCancel()
	}
	runs := make([]*repairRun, 0, len(r.runs))
	for _, run := range r.runs {
		runs = append(runs, run)
	}
	r.mu.Unlock()
	for _, run := range runs {
		run.cancel()
	}
	r.wg.Wait()
	r.mu.Lock()
	if r.events != nil {
		close(r.events)
		r.events = nil
	}
	r.mu.Unlock()
}
