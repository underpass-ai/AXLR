package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	syncDefinition = "axlr_sync"
	syncVersion    = "1.0"
	// MaxSyncRounds is how many reconciliation rounds a sync allows; MADE's
	// max_bounces of 4 is only the backstop.
	MaxSyncRounds = 2
	syncTailBytes = 2 << 10
)

// SyncResponse is what one reconciling worker did.
type SyncResponse struct {
	Task    string   `json:"task"`
	Changed []string `json:"changed"`
	Summary string   `json:"summary"`
	Notes   []string `json:"notes,omitempty"`
}

// Reconciler runs one round: a fresh worker per affected task, in turn, with
// the failing output and the other tasks' hand-backs.
type Reconciler func(ctx context.Context, round int, affected []domain.PlanTask, failing string) ([]SyncResponse, error)

// SyncOutcome is one wave's integration.
type SyncOutcome struct {
	Instance string
	Verdict  string
	Rounds   int
	Output   string
	Memory   string
}

// RunSync integrates a wave: the console runs the plan's end-to-end check;
// red opens a reconciliation round; two red rounds end BLOCKED. The sync is
// a MADE record the person can read: the workers "talk" through the notes
// and hand-backs the console relays.
func (d *CeremonyDriver) RunSync(ctx context.Context, plan domain.PlanRecord, wave int, reconcile Reconciler) (SyncOutcome, error) {
	if d == nil || d.Engine == nil || d.Checks == nil {
		return SyncOutcome{}, errors.New("MADE is not connected; the sync cannot run")
	}
	if err := d.Engine.Ready(ctx, syncDefinition, syncVersion); err != nil {
		return SyncOutcome{}, err
	}
	var tasks []domain.PlanTask
	var ids []string
	for _, t := range plan.Tasks {
		if t.Wave == wave {
			tasks = append(tasks, t)
			ids = append(ids, t.ID)
		}
	}
	about := sessionAbout(plan.Session)
	if d.Labels != nil {
		if labels, err := d.Labels.Load(ctx); err == nil && labels[plan.Session].About != "" {
			about = labels[plan.Session].About
		}
	}
	instance := fmt.Sprintf("axlr-%s-sync%d-%d", plan.ID, wave, d.now().UTC().Unix())
	inputs := map[string]string{"plan": plan.ID, "wave": fmt.Sprint(wave), "tasks": strings.Join(ids, ","), "workspace": string(plan.Workspace), "memory_about": about}
	if err := d.Engine.Start(ctx, syncDefinition, syncVersion, instance, inputs); err != nil {
		return SyncOutcome{}, fmt.Errorf("start %s %s: %w", syncDefinition, syncVersion, err)
	}
	outcome := SyncOutcome{Instance: instance}
	var responses []SyncResponse
	for round := 0; ; round++ {
		fence, err := d.Engine.Claim(ctx, instance, "integrate", fmt.Sprintf("%s:integrate:s%d", instance, round), 0)
		if err != nil {
			return outcome, fmt.Errorf("claim integrate: %w", err)
		}
		result, err := d.Checks.Run(ctx, plan.E2E)
		if err != nil {
			return outcome, err
		}
		tail := result.Output
		if len(tail) > syncTailBytes {
			tail = "…" + tail[len(tail)-syncTailBytes:]
		}
		verdict, trigger := "red", "conflict"
		switch {
		case result.Ran && result.ExitCode == 0:
			verdict, trigger = "green", "integrated"
		case !result.Ran || round >= MaxSyncRounds:
			verdict, trigger = "blocked", "exhausted"
		}
		outcome.Verdict, outcome.Rounds, outcome.Output = verdict, round, tail
		if err := d.Engine.Complete(ctx, instance, "integrate", fence, map[string]any{"verdict": verdict, "exit_code": result.ExitCode, "output_tail": tail}); err != nil {
			return outcome, fmt.Errorf("complete integrate: %w", err)
		}
		state, err := d.Engine.Transition(ctx, instance, trigger)
		if err != nil {
			return outcome, fmt.Errorf("transition %s: %w", trigger, err)
		}
		if state == "SYNCED" || state == "BLOCKED" {
			outcome.Memory = d.recordSync(ctx, plan, wave, outcome, about, responses)
			return outcome, nil
		}
		fence, err = d.Engine.Claim(ctx, instance, "reconcile", fmt.Sprintf("%s:reconcile:s%d", instance, round), 0)
		if err != nil {
			return outcome, fmt.Errorf("claim reconcile: %w", err)
		}
		affected := affectedTasks(tasks, tail)
		responses, err = reconcile(ctx, round+1, affected, tail)
		if err != nil {
			return outcome, err
		}
		if err := d.Engine.Complete(ctx, instance, "reconcile", fence, map[string]any{"reconciled": true, "responses": responses}); err != nil {
			return outcome, fmt.Errorf("complete reconcile: %w", err)
		}
		if _, err := d.Engine.Transition(ctx, instance, "reconciled"); err != nil {
			return outcome, fmt.Errorf("transition reconciled: %w", err)
		}
	}
}

// affectedTasks are the wave's tasks whose scope files the failing output
// names; when none is named, every task of the wave.
func affectedTasks(tasks []domain.PlanTask, failing string) []domain.PlanTask {
	var named []domain.PlanTask
	for _, t := range tasks {
		for _, p := range t.Scope {
			if strings.Contains(failing, p) || strings.Contains(failing, lastPathElement(p)) {
				named = append(named, t)
				break
			}
		}
	}
	if len(named) == 0 {
		return tasks
	}
	return named
}

func lastPathElement(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// SyncPacket is a reconciling worker's whole context: its own hand-back,
// the others', the notes for it and the failing output.
func SyncPacket(plan domain.PlanRecord, task domain.PlanTask, round int, failing string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sync of plan %s, round %d of %d. The end-to-end check %s %s fails after this wave. Make it pass by changing only your scope (%s), or, when another task's change conflicts with yours, leave it and say so in a line \"NOTE <task id or all>: text\".\n",
		plan.ID, round, MaxSyncRounds, plan.E2E.Program, strings.Join(plan.E2E.Args, " "), strings.Join(task.Scope, ", "))
	fmt.Fprintf(&b, "Your task %s: %s\n", task.ID, task.Goal)
	for _, other := range plan.Tasks {
		if other.Handback == nil {
			continue
		}
		who := "Task " + other.ID
		if other.ID == task.ID {
			who = "Your hand-back"
		}
		fmt.Fprintf(&b, "%s: %s (changed %s)\n", who, other.Handback.Summary, strings.Join(other.Handback.Changed, ", "))
		for _, note := range other.Handback.Notes {
			if note.To == task.ID || note.To == taskNoteRecipient {
				fmt.Fprintf(&b, "  note from %s: %s\n", note.From, note.Text)
			}
		}
	}
	fmt.Fprintf(&b, "Failing output (tail):\n%s\nEnd with a short summary of what you changed.", failing)
	return b.String()
}

func (d *CeremonyDriver) recordSync(ctx context.Context, plan domain.PlanRecord, wave int, outcome SyncOutcome, about string, responses []SyncResponse) string {
	if d.Memory == nil {
		return "not recorded: KMP is not connected"
	}
	labels := map[string][]string{"ceremony": {syncDefinition}, "plan": {plan.ID}, "wave": {fmt.Sprint(wave)}, "session": {string(plan.Session)}, "ws": {string(plan.Workspace)}}
	summary := fmt.Sprintf("Sync of plan %s wave %d ended %s after %d reconciliation rounds.", plan.ID, wave, outcome.Verdict, outcome.Rounds)
	evidence := fmt.Sprintf("MADE instance %s; e2e %s %s", outcome.Instance, plan.E2E.Program, strings.Join(plan.E2E.Args, " "))
	for _, r := range responses {
		evidence += fmt.Sprintf("\n%s changed %s: %s", r.Task, strings.Join(r.Changed, ", "), bounded(r.Summary, 300))
		for _, note := range r.Notes {
			evidence += "\n  note: " + note
		}
	}
	if outcome.Verdict != "green" {
		evidence += "\nfailing tail: " + bounded(outcome.Output, 600)
	}
	if _, err := d.Memory.RecordLinked(ctx, about, labels, MemoryRecord{ID: fmt.Sprintf("%s-sync", outcome.Instance), Kind: "observation", Summary: summary, Evidence: evidence}); err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	return "recorded in " + about
}
