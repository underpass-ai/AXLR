package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
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
	// An earlier wave's red check that names a file a later wave writes
	// waits for that wave: the end-to-end check is the plan's goal, and on
	// 20 November 2026 a two-wave plan spent two reconciliation rounds after
	// wave 1 on "resumen2.md: No such file", which wave 2 was to write, and
	// ended partial although every task was done.
	var early *CheckResult
	if wave < plan.Waves {
		result, err := d.Checks.Run(ctx, plan.E2E)
		if err != nil {
			return SyncOutcome{}, err
		}
		if result.Ran && result.ExitCode != 0 && namesLaterWave(plan, wave, result.Output) {
			return SyncOutcome{Verdict: SyncDeferred, Output: syncTail(result.Output)}, nil
		}
		early = &result
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
		var result CheckResult
		if early != nil {
			result, early = *early, nil
		} else if result, err = d.Checks.Run(ctx, plan.E2E); err != nil {
			return outcome, err
		}
		tail := syncTail(result.Output)
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
		affected := affectedTasks(tasks, tail, string(plan.Workspace))
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

// SyncDeferred is the verdict of an earlier wave whose end-to-end check
// fails on a file a later wave writes: the last wave's sync decides.
const SyncDeferred = "deferred"

// syncTail is the end of a check's output a sync keeps.
func syncTail(output string) string {
	if len(output) > syncTailBytes {
		return "…" + output[len(output)-syncTailBytes:]
	}
	return output
}

// namesLaterWave reports a failing output that names a scope file of a task
// in a wave after this one as the failure: a file the later wave creates,
// named anywhere, or one it changes, named on a line that reports an error
// or at a line number. A longer name that ends the same (stringutil.go for
// util.go) or a passing test file in a progress line deferred a real
// failure of this wave, which then had no reconciliation round (21 November
// 2026).
func namesLaterWave(plan domain.PlanRecord, wave int, failing string) bool {
	for _, line := range strings.Split(failing, "\n") {
		for _, t := range plan.Tasks {
			if t.Wave <= wave {
				continue
			}
			for _, p := range t.Scope {
				named, located := namesPath(line, p, string(plan.Workspace))
				if named && (slices.Contains(t.New, p) || located || failureLine.MatchString(line)) {
					return true
				}
			}
		}
	}
	return false
}

// failureLine is an output line that reports an error.
var failureLine = regexp.MustCompile(`(?i)error|fail|undefined|cannot|can't|no such|not found|missing|panic|traceback|exception|assert`)

// namesPath reports whether text names the file p, and whether a name is
// followed by a line number (p:12, or Python's "p", line 12). A name is a
// whole path token: p itself after any ./, p's absolute path in the
// workspace, or a shorter path that p ends with (util.go, pkg/util.go for
// svc/pkg/util.go). A longer token is another file: stringutil.go for
// util.go, or crates/b/src/lib.rs for crates/a/src/lib.rs. Until
// 21 November 2026 a top-level file after ./ or a / was never named.
func namesPath(text, p, workspace string) (named, located bool) {
	base := lastPathElement(p)
	if base == "" {
		return false, false
	}
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], base)
		if i < 0 {
			break
		}
		start, end := from+i, from+i+len(base)
		for start > 0 && pathByte(text[start-1]) {
			start--
		}
		for end < len(text) && pathByte(text[end]) {
			end++
		}
		from = end
		token := strings.TrimRight(text[start:end], ".")
		if !samePath(token, p, workspace) {
			continue
		}
		named = true
		rest := text[start+len(token):]
		if len(rest) > 1 && rest[0] == ':' && digit(rest[1]) || strings.HasPrefix(rest, `", line `) && len(rest) > 8 && digit(rest[8]) {
			return true, true
		}
	}
	return named, false
}

// samePath reports a path token that is the scope file p.
func samePath(token, p, workspace string) bool {
	for strings.HasPrefix(token, "./") || strings.HasPrefix(token, "../") {
		// A check run in a subdirectory names files from there.
		token = token[strings.IndexByte(token, '/')+1:]
	}
	if strings.HasPrefix(token, "/") {
		root := strings.TrimSuffix(workspace, "/")
		if root == "" || !strings.HasPrefix(token, root+"/") {
			return false
		}
		token = token[len(root)+1:]
	}
	return token != "" && (token == p || strings.HasSuffix(p, "/"+token))
}

func digit(b byte) bool { return b >= '0' && b <= '9' }

func pathByte(b byte) bool { return alphanumeric(b) || b == '.' || b == '_' || b == '-' || b == '/' }

func alphanumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// affectedTasks are the wave's tasks whose scope files the failing output
// names; when none is named, every task of the wave.
func affectedTasks(tasks []domain.PlanTask, failing, workspace string) []domain.PlanTask {
	var named []domain.PlanTask
	for _, t := range tasks {
		for _, p := range t.Scope {
			if mentioned, _ := namesPath(failing, p, workspace); mentioned {
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
