package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	maxTaskTestFiles  = 4
	maxTaskNotes      = 4
	maxTaskNoteText   = 500
	maxTaskQuestions  = 2
	taskDefinition    = "axlr_task"
	taskVersion       = "1.0"
	maxDigestedFile   = 1<<20 - 1
	taskNoteRecipient = "all"
)

type noteArgument struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

// taskDone are the red and green fields of axlr_step_done.
type taskDone struct {
	TestFiles  []string       `json:"test_files"`
	Untestable *bool          `json:"untestable"`
	Notes      []noteArgument `json:"notes"`
	Questions  []string       `json:"questions"`
}

// BeginTask starts one task of an approved plan in a worker session: it
// starts axlr_task, runs the console's start step (digests and the unit
// check's baseline) and claims the first model step.
func (d *CeremonyDriver) BeginTask(ctx context.Context, s *domain.Session, plan domain.PlanRecord, task domain.PlanTask) error {
	if d == nil || d.Engine == nil || d.Checks == nil || d.Files == nil {
		return errors.New("MADE is not connected; tasks cannot run")
	}
	if err := d.Engine.Ready(ctx, taskDefinition, taskVersion); err != nil {
		return err
	}
	state := s.Export()
	about := "ws:" + string(plan.Session)
	if d.Labels != nil {
		if labels, err := d.Labels.Load(ctx); err == nil && labels[plan.Session].About != "" {
			about = labels[plan.Session].About
		}
	}
	compact := d.Compact != nil && d.Compact(state.Model)
	memory := ""
	if d.Memory != nil {
		// Decision 6: besides the notes the console relays, the worker reads
		// what memory holds for its plan and task.
		if text, _, err := d.Memory.WakeFocused(ctx, about, fmt.Sprintf("plan %s task %s: notes, interfaces and decisions", plan.ID, task.ID)); err == nil && text != "" {
			memory = bounded(text, compactMemoryBytes)
		}
	}
	instance := fmt.Sprintf("axlr-%s-%d", state.ID, d.now().UTC().Unix())
	inputs := map[string]string{"plan": plan.ID, "task": task.ID, "workspace": string(state.Workspace), "memory_about": about}
	if err := d.Engine.Start(ctx, taskDefinition, taskVersion, instance, inputs); err != nil {
		return fmt.Errorf("start %s %s: %w", taskDefinition, taskVersion, err)
	}
	run := domain.CeremonyRun{Definition: taskDefinition, Version: taskVersion, Instance: instance, Step: "start", Iteration: 1, About: about, Memory: memory, Compact: compact,
		Task: &domain.TaskRun{Plan: plan.ID, Task: task.ID, Scope: slices.Clone(task.Scope), Protect: slices.Clone(task.Protect), Check: task.UnitCheck, TestFirst: task.TestFirst}}
	if err := d.claim(ctx, &run); err != nil {
		return err
	}
	start, err := d.taskDigests(ctx, run.Task)
	if err != nil {
		return err
	}
	run.Task.Start = start
	baseline, err := d.Checks.Run(ctx, task.UnitCheck)
	if err != nil {
		return err
	}
	output := map[string]any{"started": true, "test_first": task.TestFirst, "baseline_exit": baseline.ExitCode}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return fmt.Errorf("complete start: %w", err)
	}
	trigger := "test_present"
	if task.TestFirst {
		trigger = "test_first"
	}
	next, err := d.Engine.Transition(ctx, run.Instance, trigger)
	if err != nil {
		return fmt.Errorf("transition %s: %w", trigger, err)
	}
	step, ok := stateSteps[next]
	if !ok {
		return fmt.Errorf("task entered unknown state %s", next)
	}
	run.Step, run.Iteration = step, 1
	if err := d.claim(ctx, &run); err != nil {
		return err
	}
	return s.SetCeremony(run)
}

// taskDigests records the digest of every scope and protected file and,
// in a repository, of every file already changed when the task starts, so
// the earlier tasks' uncommitted work is not taken for this task's.
func (d *CeremonyDriver) taskDigests(ctx context.Context, task *domain.TaskRun) (map[string]string, error) {
	digests := map[string]string{}
	paths := append(slices.Clone(task.Scope), task.Protect...)
	changed, git, err := d.gitChanged(ctx)
	if err != nil {
		return nil, err
	}
	task.Git = git
	paths = append(paths, changed...)
	for _, p := range paths {
		if _, done := digests[p]; done {
			continue
		}
		digest, err := d.fileDigest(ctx, p)
		if err != nil {
			return nil, err
		}
		digests[p] = digest
	}
	return digests, nil
}

func (d *CeremonyDriver) fileDigest(ctx context.Context, path string) (string, error) {
	content, found, err := d.Files.Read(ctx, path, maxDigestedFile)
	if err != nil || !found {
		return "", err
	}
	return digestOf(content), nil
}

// gitChanged lists the workspace paths git reports as changed or new; ok is
// false when the workspace is not a repository. It parses the whole stdout
// of the NUL-separated form, never the tail shown to the model: every entry
// is "XY path", and a rename or copy is followed by its old path, which is
// skipped. An output the runtime cut is an error, not a shorter list.
func (d *CeremonyDriver) gitChanged(ctx context.Context) ([]string, bool, error) {
	result, err := d.Checks.Run(ctx, domain.CheckCommand{Program: "git", Args: []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}, MaxOutput: ConsoleOutputBytes})
	if err != nil {
		return nil, false, err
	}
	if !result.Ran || result.ExitCode != 0 {
		return nil, false, nil
	}
	if result.Truncated {
		return nil, false, fmt.Errorf("git status printed more than %d bytes; the task's scope cannot be checked", ConsoleOutputBytes)
	}
	var paths []string
	entries := strings.Split(result.Stdout, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		paths = append(paths, entry[3:])
		if x, y := entry[0], entry[1]; x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++ // the old path of a rename or copy
		}
	}
	sort.Strings(paths)
	return paths, true, nil
}

// gitAnswer is the trimmed stdout of a git command that exited 0, "" when it
// did not run or failed: stderr, and the stdout of a failed command (rev-parse
// echoes HEAD in a repository without commits), are not an answer to record.
func gitAnswer(ctx context.Context, checks CheckRunnerPort, args ...string) string {
	result, err := checks.Run(ctx, domain.CheckCommand{Program: "git", Args: args})
	if err != nil || !result.Ran || result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// changedByTask lists the paths this task changed: in a repository, those
// git reports whose digest differs from the start; otherwise the scope
// files whose digest differs.
func (d *CeremonyDriver) changedByTask(ctx context.Context, task *domain.TaskRun) ([]string, error) {
	candidates := slices.Clone(task.Scope)
	if task.Git {
		changed, _, err := d.gitChanged(ctx)
		if err != nil {
			return nil, err
		}
		candidates = changed
	}
	var out []string
	for _, p := range candidates {
		now, err := d.fileDigest(ctx, p)
		if err != nil {
			return nil, err
		}
		if before, known := task.Start[p]; !known || before != now {
			out = append(out, p)
		}
	}
	return out, nil
}

// taskRed accepts the failing test the worker wrote, or its declaration
// that the task cannot be tested.
func (d *CeremonyDriver) taskRed(ctx context.Context, run *domain.CeremonyRun, done stepDone, report map[string]any) (map[string]any, string, bool, string, error) {
	task := run.Task
	if done.Untestable != nil && *done.Untestable {
		if strings.TrimSpace(done.Observed) == "" {
			return nil, "", false, "untestable needs observed: why no test can fail first", nil
		}
		return map[string]any{"red": false, "untestable": true, "observed": done.Observed, "settled": true}, "untestable", false, "", nil
	}
	if len(done.TestFiles) == 0 || len(done.TestFiles) > maxTaskTestFiles || strings.TrimSpace(done.Expected) == "" {
		return nil, "", false, fmt.Sprintf("red needs test_files (1 to %d paths in the scope) and expected, or untestable=true with observed", maxTaskTestFiles), nil
	}
	frozen := map[string]string{}
	for _, raw := range done.TestFiles {
		p, ok := workspacePath(raw)
		if !ok || !slices.Contains(task.Scope, p) {
			return nil, "", false, fmt.Sprintf("test file %s is not in the task's scope (%s)", raw, strings.Join(task.Scope, ", ")), nil
		}
		digest, err := d.fileDigest(ctx, p)
		if err != nil {
			return nil, "", false, "", err
		}
		if digest == "" || digest == task.Start[p] {
			return nil, "", false, fmt.Sprintf("test file %s is unchanged since the task started; write the failing test first", p), nil
		}
		frozen[p] = digest
	}
	result, err := d.Checks.Run(ctx, task.Check)
	if err != nil {
		return nil, "", false, "", err
	}
	red := result.Ran && result.ExitCode != 0
	output := map[string]any{"red": red, "settled": red, "test_files": done.TestFiles, "expected": done.Expected}
	addEvidence(output, report, task.Check, result)
	if tail := result.Output; strings.TrimSpace(done.Expected) != "" && !strings.Contains(tail, strings.TrimSpace(done.Expected)) {
		report["warning"] = "the check output does not show the expected text; it is kept as evidence, not as a verdict"
	}
	switch {
	case red:
		task.Frozen = frozen
		return output, "test_failing", false, "", nil
	case run.Iteration >= ceremonyRepeatLimit:
		return output, "red_exhausted", false, "", nil
	}
	report["feedback"] = "the unit check passes, so the test does not fail yet; make it fail for the missing behaviour"
	if result.RanNoTests() {
		report["feedback"] = noTestsFeedback
	}
	return output, "", true, "", nil
}

// taskGreen accepts the change when the frozen tests and protected files are
// intact, every change is inside the scope and the unit check passes.
func (d *CeremonyDriver) taskGreen(ctx context.Context, s domain.Session, run *domain.CeremonyRun, done stepDone, report map[string]any) (map[string]any, string, bool, string, error) {
	task := run.Task
	if strings.TrimSpace(done.Summary) == "" || strings.TrimSpace(done.SummaryEN) == "" {
		return nil, "", false, "green needs summary and summary_en", nil
	}
	if len(done.Notes) > maxTaskNotes || len(done.Questions) > maxTaskQuestions {
		return nil, "", false, fmt.Sprintf("at most %d notes and %d questions", maxTaskNotes, maxTaskQuestions), nil
	}
	for _, note := range done.Notes {
		if strings.TrimSpace(note.Text) == "" || len(note.Text) > maxTaskNoteText || (note.To != taskNoteRecipient && !domain.ValidSlug(note.To)) {
			return nil, "", false, fmt.Sprintf("a note is {to: a task id or all, text: at most %d characters}", maxTaskNoteText), nil
		}
	}
	for p, frozen := range task.Frozen {
		if now, err := d.fileDigest(ctx, p); err != nil {
			return nil, "", false, "", err
		} else if now != frozen {
			return nil, "", false, fmt.Sprintf("test file %s changed after red; restore it and make the code pass the test as written", p), nil
		}
	}
	for _, p := range task.Protect {
		if now, err := d.fileDigest(ctx, p); err != nil {
			return nil, "", false, "", err
		} else if now != task.Start[p] {
			return nil, "", false, fmt.Sprintf("protected file %s changed; restore it", p), nil
		}
	}
	changed, err := d.changedByTask(ctx, task)
	if err != nil {
		return nil, "", false, "", err
	}
	var outside []string
	for _, p := range changed {
		if !slices.Contains(task.Scope, p) {
			outside = append(outside, p)
		}
	}
	if len(outside) > 0 {
		return nil, "", false, fmt.Sprintf("changes outside the task's scope: %s; undo them (the scope is %s)", strings.Join(outside, ", "), strings.Join(task.Scope, ", ")), nil
	}
	result, err := d.Checks.Run(ctx, task.Check)
	if err != nil {
		return nil, "", false, "", err
	}
	green := result.Ran && result.ExitCode == 0 && !result.RanNoTests()
	output := map[string]any{"green": green, "summary": done.Summary, "summary_en": bounded(done.SummaryEN, 1500)}
	if len(done.Notes) > 0 {
		output["notes"] = done.Notes
	}
	if len(done.Questions) > 0 {
		output["questions"] = done.Questions
	}
	addEvidence(output, report, task.Check, result)
	if green {
		if err := d.recordHandback(ctx, task, done); err != nil {
			return nil, "", false, "", err
		}
		return output, "check_passing", false, "", nil
	}
	if run.Iteration >= ceremonyRepeatLimit {
		return output, "green_exhausted", false, "", nil
	}
	report["feedback"] = "the unit check still fails; fix what its output shows"
	if result.RanNoTests() {
		report["feedback"] = noTestsFeedback
	}
	return output, "", true, "", nil
}

// recordHandback keeps the worker's summary, notes and questions in the
// plan registry for the later tasks and the sync.
func (d *CeremonyDriver) recordHandback(ctx context.Context, task *domain.TaskRun, done stepDone) error {
	if d.Plans == nil {
		return nil
	}
	record, err := d.planRecord(ctx, task.Plan)
	if err != nil {
		return err
	}
	t, ok := record.Task(task.Task)
	if !ok {
		return fmt.Errorf("task %s is not in plan %s", task.Task, task.Plan)
	}
	handback := &domain.TaskHandback{Summary: bounded(done.Summary, 2000), SummaryEN: bounded(done.SummaryEN, 1500), Questions: done.Questions}
	for _, note := range done.Notes {
		handback.Notes = append(handback.Notes, domain.TaskNote{From: task.Task, To: note.To, Text: note.Text})
	}
	t.Handback, record.Updated = handback, d.now()
	return d.Plans.Save(ctx, record)
}

// taskHandback is the console's last step: what changed and at which
// revision.
func (d *CeremonyDriver) taskHandback(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	if err := d.claim(ctx, &run); err != nil {
		return StepResult{}, err
	}
	task := run.Task
	changed, err := d.changedByTask(ctx, task)
	if err != nil {
		return StepResult{}, err
	}
	revision := ""
	if task.Git {
		revision = gitAnswer(ctx, d.Checks, "rev-parse", "HEAD")
	}
	output := map[string]any{"done": true, "changed_files": changed, "revision": revision, "scope_checked_by": "git status"}
	if !task.Git {
		output["scope_checked_by"] = "digests only: the workspace is not a git repository, so changes outside the scope were not detected"
	}
	if d.Plans != nil {
		if record, err := d.planRecord(ctx, task.Plan); err == nil {
			if t, ok := record.Task(task.Task); ok && t.Handback != nil {
				t.Handback.Changed, t.Handback.Revision = changed, revision
				_ = d.Plans.Save(ctx, record)
			}
		}
	}
	report["changed_files"] = changed
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("complete handback: %w", err))
	}
	state, err := d.Engine.Transition(ctx, run.Instance, "handed_back")
	if err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("transition handed_back: %w", err))
	}
	return d.enter(ctx, s, run, state, output, report)
}

// finishTask records a task's terminal state in the registry and memory.
func (d *CeremonyDriver) finishTask(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string, output map[string]any) string {
	task := run.Task
	status, reason := domain.TaskDone, ""
	if state != "DONE" {
		status = domain.TaskBlocked
		reason = fmt.Sprintf("ended %s at step %s", state, run.Step)
		if observed, _ := output["observed"].(string); observed != "" {
			reason += ": " + bounded(observed, 400)
		}
	}
	var handback *domain.TaskHandback
	wave := 0
	if d.Plans != nil {
		if record, err := d.planRecord(ctx, task.Plan); err == nil {
			if t, ok := record.Task(task.Task); ok {
				t.Status, t.Reason, t.Instance, t.Step, t.Session = status, reason, run.Instance, run.Step, s.Export().ID
				handback, wave = t.Handback, t.Wave
				record.Updated = d.now()
				_ = d.Plans.Save(ctx, record)
			}
		}
	}
	if d.Memory == nil {
		return "not recorded: KMP is not connected"
	}
	labels := map[string][]string{"ceremony": {run.Definition}, "plan": {task.Plan}, "task": {task.Task}, "wave": {fmt.Sprint(wave)}, "session": {string(s.Export().ID)}, "ws": {string(s.Export().Workspace)}}
	kind, summary := "observation", fmt.Sprintf("Task %s of plan %s %s.", task.Task, task.Plan, strings.ToLower(string(status)))
	evidence := fmt.Sprintf("MADE instance %s; check %s %s", run.Instance, task.Check.Program, strings.Join(task.Check.Args, " "))
	if reason != "" {
		summary += " " + reason
	}
	if handback != nil {
		if status == domain.TaskDone {
			kind = "success_path"
		}
		summary += " " + handback.SummaryEN
		for _, note := range handback.Notes {
			evidence += fmt.Sprintf("\nnote from %s to %s: %s", note.From, note.To, note.Text)
		}
		if len(handback.Changed) > 0 {
			evidence += "\nchanged: " + strings.Join(handback.Changed, ", ")
		}
	}
	if _, err := d.Memory.RecordLinked(ctx, run.About, labels, MemoryRecord{ID: run.Instance + "-handback", Kind: kind, Summary: summary, Evidence: evidence}); err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	return "recorded in " + run.About
}

// TaskPrompt is the worker's first message: the context pack and the notes
// earlier tasks addressed to this one or to all.
func TaskPrompt(plan domain.PlanRecord, task domain.PlanTask) root.Text {
	var notes []string
	for _, other := range plan.Tasks {
		if other.Handback == nil || other.ID == task.ID {
			continue
		}
		for _, note := range other.Handback.Notes {
			if note.To == task.ID || note.To == taskNoteRecipient {
				notes = append(notes, fmt.Sprintf("- from %s: %s", note.From, note.Text))
			}
		}
	}
	text := task.Pack
	if len(notes) > 0 {
		text += "Notes from earlier tasks:\n" + strings.Join(notes, "\n") + "\n"
	}
	return root.Text(text)
}
