package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// PlanWorkbench is a RepairWorkbench rooted at the workspace that can also
// start a task ceremony and run a sync.
type PlanWorkbench interface {
	RepairWorkbench
	BeginTask(ctx context.Context, s *domain.Session, plan domain.PlanRecord, task domain.PlanTask) error
	RunSync(ctx context.Context, plan domain.PlanRecord, wave int, reconcile Reconciler) (SyncOutcome, error)
	// Digest is the SHA-256 of a workspace file, "" when it does not exist.
	Digest(ctx context.Context, path string) string
	// ReadFile and WriteFile let the runner put a blocked task's scope back.
	ReadFile(ctx context.Context, path string) ([]byte, bool, error)
	WriteFile(ctx context.Context, path string, content []byte) error
}

// PlanWorkbenchPort opens a workbench in the plan's workspace.
type PlanWorkbenchPort interface {
	Open(ctx context.Context, workspace string) (PlanWorkbench, error)
}

// PlanEvent tells the panel a plan changed.
type PlanEvent struct{ Plan domain.PlanRecord }

// PlanRunner runs approved plans in the background: the tasks of a wave one
// after another in the shared workspace, each in a fresh session that the
// console drives like a self-repair, then the wave's sync. One local GPU
// serves one model at a time, so parallel workers would only queue on it.
type PlanRunner struct {
	Plans     PlanRegistryPort
	Store     SessionStorePort
	Workbench PlanWorkbenchPort
	// RunToken names this console launch; a plan found running under
	// another token was interrupted.
	RunToken string
	Lifetime context.Context
	Now      func() time.Time
	NewID    func() (domain.SessionID, error)

	mu      sync.Mutex
	running map[string]context.CancelFunc
	events  chan PlanEvent
	closed  bool
	wg      sync.WaitGroup
	// unsaved is the plan changes the registry refused since Records last
	// reported them: the registry cannot show those itself.
	unsaved error
}

func (r *PlanRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *PlanRunner) newID() (domain.SessionID, error) {
	if r.NewID != nil {
		return r.NewID()
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return domain.NewSessionID(hex.EncodeToString(raw[:]))
}

// Events delivers plan changes to the panel; a full buffer drops events,
// the panel reloads from the registry anyway.
func (r *PlanRunner) Events() <-chan PlanEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events == nil {
		r.events = make(chan PlanEvent, 64)
	}
	return r.events
}

func (r *PlanRunner) publish(record domain.PlanRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.events == nil {
		r.events = make(chan PlanEvent, 64)
	}
	select {
	case r.events <- PlanEvent{Plan: record}:
	default:
	}
}

// Records lists the plans, newest first. It also reports, once, the plan
// changes the registry refused since the last call, so the panel shows them.
func (r *PlanRunner) Records(ctx context.Context) ([]domain.PlanRecord, error) {
	r.mu.Lock()
	unsaved := r.unsaved
	r.unsaved = nil
	r.mu.Unlock()
	records, err := r.Plans.Load(ctx)
	if err != nil {
		return nil, errors.Join(err, unsaved)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Updated.After(records[j].Updated) })
	return records, unsaved
}

// Start runs a ready plan in the background.
func (r *PlanRunner) Start(ctx context.Context, planID string) error {
	record, err := r.load(ctx, planID)
	if err != nil {
		return err
	}
	if record.Status != domain.PlanReady && record.Status != domain.PlanInterrupted {
		return fmt.Errorf("plan %s is %s, not ready", planID, record.Status)
	}
	lifetime := r.Lifetime
	if lifetime == nil {
		lifetime = context.Background()
	}
	runCtx, cancel := context.WithCancel(lifetime)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return errors.New("the console is closing; the plan was not started")
	}
	if r.running == nil {
		r.running = map[string]context.CancelFunc{}
	}
	if _, busy := r.running[planID]; busy {
		r.mu.Unlock()
		cancel()
		return fmt.Errorf("plan %s is already running", planID)
	}
	r.running[planID] = cancel
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		defer cancel()
		defer func() {
			r.mu.Lock()
			delete(r.running, planID)
			r.mu.Unlock()
		}()
		r.run(runCtx, planID)
	}()
	return nil
}

// Reconcile marks plans a stopped console left running as interrupted.
func (r *PlanRunner) Reconcile(ctx context.Context) error {
	records, err := r.Plans.Load(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Status == domain.PlanRunning && record.RunToken != r.RunToken {
			record.Status, record.Error, record.Updated = domain.PlanInterrupted, "the console that ran it stopped; start it again from /plan", r.now()
			for i := range record.Tasks {
				if record.Tasks[i].Status == domain.TaskRunning {
					record.Tasks[i].Status, record.Tasks[i].Reason = domain.TaskPending, "interrupted"
				}
			}
			if err := r.Plans.Save(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

// Wait blocks until every running plan has ended on its own.
func (r *PlanRunner) Wait() { r.wg.Wait() }

// Close stops every plan and waits for them.
func (r *PlanRunner) Close() {
	r.mu.Lock()
	r.closed = true
	for _, cancel := range r.running {
		cancel()
	}
	r.mu.Unlock()
	r.wg.Wait()
	r.mu.Lock()
	if r.events != nil {
		close(r.events)
		r.events = nil
	}
	r.mu.Unlock()
}

func (r *PlanRunner) load(ctx context.Context, id string) (domain.PlanRecord, error) {
	records, err := r.Plans.Load(ctx)
	if err != nil {
		return domain.PlanRecord{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return domain.PlanRecord{}, fmt.Errorf("plan %s is not in the registry", id)
}

// update reloads the plan, applies change and saves it, so the driver's
// writes from inside a task are never lost. A change the registry refuses is
// not published as if it were saved: it is kept for Records to report, and
// the panel is told to reload so it does.
func (r *PlanRunner) update(ctx context.Context, id string, change func(*domain.PlanRecord)) domain.PlanRecord {
	ctx = context.WithoutCancel(ctx)
	record, err := r.load(ctx, id)
	if err != nil {
		return record
	}
	change(&record)
	record.Updated = r.now()
	if err := r.Plans.Save(ctx, record); err != nil {
		r.mu.Lock()
		r.unsaved = errors.Join(r.unsaved, fmt.Errorf("plan %s: the registry did not save its latest change: %w", id, err))
		r.mu.Unlock()
		if stored, err := r.load(ctx, id); err == nil {
			r.publish(stored)
		}
		return record
	}
	r.publish(record)
	return record
}

func (r *PlanRunner) run(ctx context.Context, id string) {
	record := r.update(ctx, id, func(p *domain.PlanRecord) { p.Status, p.RunToken, p.Error = domain.PlanRunning, r.RunToken, "" })
	workbench, err := r.Workbench.Open(ctx, string(record.Workspace))
	if err != nil {
		r.update(ctx, id, func(p *domain.PlanRecord) {
			p.Status, p.Error = domain.PlanInterrupted, bounded("open the workspace: "+err.Error(), 600)
		})
		return
	}
	defer workbench.Close()
	for wave := 1; wave <= record.Waves; wave++ {
		record, _ = r.load(ctx, id)
		for _, task := range record.Tasks {
			if task.Wave != wave || task.Status == domain.TaskDone {
				continue
			}
			if blocker := blockedDependency(record, task); blocker != "" {
				r.update(ctx, id, func(p *domain.PlanRecord) {
					if t, ok := p.Task(task.ID); ok {
						t.Status, t.Reason = domain.TaskSkipped, "depends on "+blocker+", which did not finish"
					}
				})
				continue
			}
			r.runTask(ctx, workbench, id, task.ID)
			if ctx.Err() != nil {
				r.update(ctx, id, func(p *domain.PlanRecord) { p.Status, p.Error = domain.PlanInterrupted, "interrupted" })
				return
			}
		}
		record, _ = r.load(ctx, id)
		if !waveFinished(record, wave) {
			continue // a blocked wave is not integrated; its dependents are skipped
		}
		outcome, err := workbench.RunSync(ctx, record, wave, r.reconciler(ctx, workbench, id))
		r.update(ctx, id, func(p *domain.PlanRecord) {
			s := domain.SyncRecord{Wave: wave, Instance: outcome.Instance, Rounds: outcome.Rounds, Verdict: outcome.Verdict, Output: bounded(outcome.Output, 2000)}
			if err != nil {
				s.Verdict, s.Output = "blocked", bounded(err.Error(), 600)
			}
			p.Syncs = append(p.Syncs, s)
		})
		if ctx.Err() != nil {
			r.update(ctx, id, func(p *domain.PlanRecord) { p.Status, p.Error = domain.PlanInterrupted, "interrupted" })
			return
		}
	}
	r.update(ctx, id, func(p *domain.PlanRecord) {
		p.Status = domain.PlanDone
		for _, t := range p.Tasks {
			if t.Status != domain.TaskDone {
				p.Status = domain.PlanPartial
			}
		}
		for _, s := range p.Syncs {
			if s.Verdict != "green" {
				p.Status = domain.PlanPartial
			}
		}
	})
}

func blockedDependency(plan domain.PlanRecord, task domain.PlanTask) string {
	for _, dep := range task.DependsOn {
		if other, ok := plan.Task(dep); ok && other.Status != domain.TaskDone {
			return dep
		}
	}
	return ""
}

func waveFinished(plan domain.PlanRecord, wave int) bool {
	any := false
	for _, t := range plan.Tasks {
		if t.Wave == wave {
			any = true
			if t.Status != domain.TaskDone {
				return false
			}
		}
	}
	return any
}

const maxTaskRetries = 2

// runTask drives one worker session: the task ceremony, the turns, and the
// rule for small models that stop using their tools: after the console's one
// reminder, a second reply that leaves the step open ends the task BLOCKED.
func (r *PlanRunner) runTask(ctx context.Context, workbench PlanWorkbench, planID, taskID string) {
	record, err := r.load(ctx, planID)
	if err != nil {
		return
	}
	task, _ := record.Task(taskID)
	fail := func(reason string) {
		r.update(ctx, planID, func(p *domain.PlanRecord) {
			if t, ok := p.Task(taskID); ok && t.Status != domain.TaskDone {
				t.Status, t.Reason = domain.TaskBlocked, bounded(reason, 600)
			}
		})
	}
	id, err := r.newID()
	if err != nil {
		fail(err.Error())
		return
	}
	child, err := domain.NewSession(id, record.Workspace, root.ModelID(record.Worker))
	if err == nil {
		err = child.SetMode(domain.ModeTask)
	}
	if err != nil {
		fail(err.Error())
		return
	}
	r.update(ctx, planID, func(p *domain.PlanRecord) {
		if t, ok := p.Task(taskID); ok {
			t.Status, t.Session, t.Reason = domain.TaskRunning, id, ""
		}
	})
	// A task that does not finish must not leave half its work behind: the
	// next tasks and the sync would build on it. Its scope files are put back
	// as they were; files it created are named, not deleted.
	saved := map[string][]byte{}
	for _, p := range task.Scope {
		if content, found, err := workbench.ReadFile(ctx, p); err == nil && found {
			saved[p] = content
		}
	}
	defer func() {
		current, err := r.load(ctx, planID)
		if err != nil {
			return
		}
		t, ok := current.Task(taskID)
		if !ok || t.Status == domain.TaskDone {
			return
		}
		var restored, created []string
		for _, p := range task.Scope {
			if original, existed := saved[p]; existed {
				if now, _, _ := workbench.ReadFile(ctx, p); string(now) != string(original) && workbench.WriteFile(context.WithoutCancel(ctx), p, original) == nil {
					restored = append(restored, p)
				}
			} else if workbench.Digest(ctx, p) != "" {
				created = append(created, p)
			}
		}
		if len(restored)+len(created) == 0 {
			return
		}
		r.update(ctx, planID, func(p *domain.PlanRecord) {
			if t, ok := p.Task(taskID); ok {
				if len(restored) > 0 {
					t.Reason += "; restored " + strings.Join(restored, ", ")
				}
				if len(created) > 0 {
					t.Reason += "; left the new files " + strings.Join(created, ", ")
				}
			}
		})
	}()
	if err := workbench.BeginTask(ctx, &child, record, *task); err != nil {
		fail("start: " + err.Error())
		return
	}
	if err := r.Store.Save(ctx, child); err != nil {
		fail(err.Error())
		return
	}
	err = workbench.Begin(ctx, &child, TaskPrompt(record, *task), nil)
	retries := 0
	for {
		if ctx.Err() != nil {
			return
		}
		r.update(ctx, planID, func(p *domain.PlanRecord) {
			if t, ok := p.Task(taskID); ok {
				if run, live := child.Ceremony(); live {
					t.Step, t.Instance = run.Step, run.Instance
				}
			}
		})
		if err != nil {
			if retries++; retries > maxTaskRetries {
				fail(err.Error())
				return
			}
			err = workbench.Continue(ctx, &child, nil)
			continue
		}
		run, live := child.Ceremony()
		switch {
		case !live:
			// The driver recorded DONE or BLOCKED in the registry.
			current, _ := r.load(ctx, planID)
			if t, ok := current.Task(taskID); ok && t.Status == domain.TaskRunning {
				fail("the task session ended without a ceremony outcome")
			}
			return
		case child.Status() == domain.StatusApproval && len(child.Pending()) > 0:
			fail("the worker's call " + string(child.Pending()[0].Call.Name) + " needs the person's approval; workers run without the person")
			return
		case child.Status() == domain.StatusComplete:
			fail(fmt.Sprintf("the model did not use its tools: it ended its turn twice with step %s open; when its answer shows a tool call as text, check the server's tool-call parser", run.Step))
			return
		default:
			if retries++; retries > maxTaskRetries {
				fail("the task session stalled in state " + string(child.Status()))
				return
			}
			err = workbench.Continue(ctx, &child, nil)
		}
	}
}

var noteLine = regexp.MustCompile(`(?m)^NOTE ([a-z0-9-]+|all):\s*(.+)$`)

// reconciler runs each affected task's fresh worker with its sync packet and
// collects what it changed, its summary and its notes.
func (r *PlanRunner) reconciler(ctx context.Context, workbench PlanWorkbench, planID string) Reconciler {
	return func(ctx context.Context, round int, affected []domain.PlanTask, failing string) ([]SyncResponse, error) {
		var responses []SyncResponse
		for _, task := range affected {
			plan, err := r.load(ctx, planID)
			if err != nil {
				return responses, err
			}
			id, err := r.newID()
			if err != nil {
				return responses, err
			}
			child, err := domain.NewSession(id, plan.Workspace, root.ModelID(plan.Worker))
			if err == nil {
				err = child.SetMode(domain.ModeTask)
			}
			if err != nil {
				return responses, err
			}
			before := scopeSnapshot(ctx, workbench, task)
			if err := r.Store.Save(ctx, child); err != nil {
				return responses, err
			}
			response := SyncResponse{Task: task.ID}
			if err := workbench.Begin(ctx, &child, root.Text(SyncPacket(plan, task, round, failing)), nil); err != nil {
				response.Summary = "the worker failed: " + bounded(err.Error(), 300)
			} else if messages := child.Messages(); len(messages) > 0 {
				last := string(messages[len(messages)-1].Content)
				response.Summary = bounded(last, 1000)
				for _, match := range noteLine.FindAllStringSubmatch(last, maxTaskNotes) {
					response.Notes = append(response.Notes, match[1]+": "+bounded(match[2], maxTaskNoteText))
				}
			}
			after := scopeSnapshot(ctx, workbench, task)
			for _, p := range task.Scope {
				if before[p] != after[p] {
					response.Changed = append(response.Changed, p)
				}
			}
			if response.Summary == "" {
				response.Summary = "the worker changed nothing and left no note"
			}
			responses = append(responses, response)
		}
		return responses, nil
	}
}

// scopeSnapshot is the digest of each scope file of a task.
func scopeSnapshot(ctx context.Context, workbench PlanWorkbench, task domain.PlanTask) map[string]string {
	out := map[string]string{}
	for _, p := range task.Scope {
		out[p] = workbench.Digest(ctx, p)
	}
	return out
}

// PlanStarter starts a plan once it is READY.
type PlanStarter interface {
	Start(ctx context.Context, planID string) error
}

var _ PlanStarter = (*PlanRunner)(nil)
