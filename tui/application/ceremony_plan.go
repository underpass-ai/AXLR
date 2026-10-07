package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// Limits of a plan: what makes a task atomic for a small model.
const (
	maxPlanTasks        = 12
	maxTaskScope        = 8
	maxTaskCitations    = 8
	maxTaskProtect      = 8
	maxPlanInterfaces   = 2 << 10
	maxTaskPack         = 12 << 10
	citationMargin      = 20
	minCitationQuote    = 12
	maxCitedFile        = 1<<20 - 1
	maxPlanDefectsShown = 12
)

type citationArgument struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Quote string `json:"quote"`
}

type taskArgument struct {
	ID        string             `json:"id"`
	Goal      string             `json:"goal"`
	Scope     []string           `json:"scope"`
	Context   []citationArgument `json:"context"`
	UnitCheck *commandArgument   `json:"unit_check"`
	DependsOn []string           `json:"depends_on"`
	TestFirst bool               `json:"test_first"`
	Protect   []string           `json:"protect"`
}

type commandArgument struct {
	Program string   `json:"program"`
	Args    []string `json:"args"`
}

func (c *commandArgument) command() (domain.CheckCommand, bool) {
	if c == nil || strings.TrimSpace(c.Program) == "" {
		return domain.CheckCommand{}, false
	}
	return domain.CheckCommand{Program: c.Program, Args: append([]string(nil), c.Args...)}, true
}

// planDone are the decompose fields of axlr_step_done.
type planDone struct {
	Tasks      []taskArgument   `json:"tasks"`
	E2ECheck   *commandArgument `json:"e2e_check"`
	Interfaces string           `json:"interfaces"`
}

// proposalCommands lists every command a decompose hand-back would run.
func (p planDone) commands() []domain.CheckCommand {
	var out []domain.CheckCommand
	if command, ok := p.E2ECheck.command(); ok {
		out = append(out, command)
	}
	for _, task := range p.Tasks {
		if command, ok := task.UnitCheck.command(); ok {
			out = append(out, command)
		}
	}
	return out
}

// planVerification is the console's verdict on one proposal.
type planVerification struct {
	tasks    []domain.PlanTask
	e2e      domain.CheckCommand
	baseline int
	waves    int
	defects  []string
}

var whitespace = regexp.MustCompile(`\s+`)

func collapse(text string) string { return strings.TrimSpace(whitespace.ReplaceAllString(text, " ")) }

// verifyPlan checks a proposal mechanically, without a model: ids,
// dependencies and waves, scopes, citations, commands and the size of every
// context pack. It runs each check command once, as the baseline.
func (d *CeremonyDriver) verifyPlan(ctx context.Context, slug string, done stepDone) (planVerification, error) {
	var v planVerification
	defect := func(format string, args ...any) { v.defects = append(v.defects, fmt.Sprintf(format, args...)) }
	proposal := done.planDone
	switch {
	case len(proposal.Tasks) == 0:
		defect("plan: tasks is empty; propose 1 to %d tasks", maxPlanTasks)
	case len(proposal.Tasks) > maxPlanTasks:
		defect("plan: %d tasks; make a second plan for the rest (at most %d)", len(proposal.Tasks), maxPlanTasks)
	}
	if len(proposal.Interfaces) > maxPlanInterfaces {
		defect("plan: interfaces is %d bytes; keep it under %d", len(proposal.Interfaces), maxPlanInterfaces)
	}
	if strings.TrimSpace(done.SummaryEN) == "" {
		defect("plan: summary_en is missing")
	}
	ids := map[string]int{}
	for i, task := range proposal.Tasks {
		if !domain.ValidSlug(task.ID) {
			defect("task %d: id %q must be a slug (lowercase letters, digits and dashes)", i+1, task.ID)
		} else if _, seen := ids[task.ID]; seen {
			defect("task %s: duplicate id", task.ID)
		}
		ids[task.ID] = i
	}
	for _, task := range proposal.Tasks {
		if strings.TrimSpace(task.Goal) == "" {
			defect("task %s: goal is empty", task.ID)
		}
		for _, dep := range task.DependsOn {
			if _, ok := ids[dep]; !ok {
				defect("task %s: depends on unknown task %q", task.ID, dep)
			}
		}
	}
	waves, cyclic := planWaves(proposal.Tasks)
	if len(cyclic) > 0 {
		defect("tasks %s: their dependencies form a cycle", strings.Join(cyclic, ", "))
	}
	tasks := make([]domain.PlanTask, len(proposal.Tasks))
	for i, task := range proposal.Tasks {
		t := domain.PlanTask{ID: task.ID, Goal: collapse(task.Goal), DependsOn: append([]string(nil), task.DependsOn...), TestFirst: task.TestFirst, Wave: waves[task.ID], Status: domain.TaskPending}
		switch {
		case len(task.Scope) == 0:
			defect("task %s: scope is empty", task.ID)
		case len(task.Scope) > maxTaskScope:
			defect("task %s: scope lists %d files; split the task (at most %d)", task.ID, len(task.Scope), maxTaskScope)
		}
		for _, raw := range task.Scope {
			clean, ok := workspacePath(raw)
			if !ok {
				defect("task %s: scope path %q is outside the workspace", task.ID, raw)
				continue
			}
			t.Scope = append(t.Scope, clean)
			if _, found, err := d.Files.Read(ctx, clean, 1); err != nil {
				return v, err
			} else if !found {
				t.New = append(t.New, clean)
			}
		}
		if len(task.Protect) > maxTaskProtect {
			defect("task %s: protect lists %d files (at most %d)", task.ID, len(task.Protect), maxTaskProtect)
		}
		for _, raw := range task.Protect {
			if clean, ok := workspacePath(raw); ok {
				t.Protect = append(t.Protect, clean)
			} else {
				defect("task %s: protected path %q is outside the workspace", task.ID, raw)
			}
		}
		if len(task.Context) > maxTaskCitations {
			defect("task %s: context cites %d places (at most %d)", task.ID, len(task.Context), maxTaskCitations)
		}
		for _, cite := range task.Context {
			t.Context = append(t.Context, domain.Citation{Path: cite.Path, Line: cite.Line, Quote: cite.Quote})
		}
		command, ok := task.UnitCheck.command()
		if !ok {
			defect("task %s: unit_check {program, args} is missing", task.ID)
		} else {
			t.UnitCheck = command
		}
		tasks[i] = t
	}
	// Scopes of one wave must be disjoint: its tasks run one after another
	// in the same workspace and must not undo each other.
	owner := map[string]string{}
	for _, t := range tasks {
		for _, p := range t.Scope {
			key := fmt.Sprintf("%d\x00%s", t.Wave, p)
			if other, taken := owner[key]; taken && other != t.ID {
				defect("tasks %s and %s: both change %s in wave %d; give it to one of them or order them with depends_on", other, t.ID, p, t.Wave)
			}
			owner[key] = t.ID
		}
	}
	if len(v.defects) > 0 {
		v.defects = boundDefects(v.defects)
		return v, nil
	}
	// Citations and packs need the files; commands run only on a proposal
	// whose structure holds.
	for i := range tasks {
		excerpts, problems, err := d.citedExcerpts(ctx, tasks[i])
		if err != nil {
			return v, err
		}
		v.defects = append(v.defects, problems...)
		tasks[i].Pack = taskPack(slug, tasks[i], proposal.Interfaces, excerpts)
		if len(tasks[i].Pack) > maxTaskPack {
			defect("task %s: its context pack is %d bytes (at most %d); split the task or cite less", tasks[i].ID, len(tasks[i].Pack), maxTaskPack)
		}
	}
	if len(v.defects) > 0 {
		v.defects = boundDefects(v.defects)
		return v, nil
	}
	for i := range tasks {
		result, err := d.Checks.Run(ctx, tasks[i].UnitCheck)
		if err != nil {
			return v, err
		}
		if !result.Ran {
			defect("task %s: unit_check did not run (%s); give program and args separately, with a program that exists", tasks[i].ID, bounded(result.Output, 200))
			continue
		}
		tasks[i].Baseline = result.ExitCode
	}
	e2e, ok := proposal.E2ECheck.command()
	if !ok {
		defect("plan: e2e_check {program, args} is missing")
	} else {
		result, err := d.Checks.Run(ctx, e2e)
		if err != nil {
			return v, err
		}
		if !result.Ran {
			defect("plan: e2e_check did not run (%s)", bounded(result.Output, 200))
		}
		v.e2e, v.baseline = e2e, result.ExitCode
	}
	v.tasks, v.defects = tasks, boundDefects(v.defects)
	for _, t := range tasks {
		v.waves = max(v.waves, t.Wave)
	}
	return v, nil
}

func boundDefects(defects []string) []string {
	if len(defects) > maxPlanDefectsShown {
		return append(defects[:maxPlanDefectsShown:maxPlanDefectsShown], fmt.Sprintf("and %d more defects", len(defects)-maxPlanDefectsShown))
	}
	return defects
}

// workspacePath cleans a workspace-relative path and refuses one that
// leaves the workspace.
func workspacePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") {
		return "", false
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// planWaves orders tasks by their dependencies: a task's wave is one more
// than the latest wave it depends on. Tasks in a cycle are returned.
func planWaves(tasks []taskArgument) (map[string]int, []string) {
	deps := map[string][]string{}
	for _, task := range tasks {
		deps[task.ID] = task.DependsOn
	}
	wave := map[string]int{}
	for len(wave) < len(deps) {
		progressed := false
		for id, needs := range deps {
			if _, done := wave[id]; done {
				continue
			}
			level, ready := 1, true
			for _, dep := range needs {
				w, done := wave[dep]
				if _, known := deps[dep]; !known {
					continue // reported as an unknown dependency
				}
				if !done {
					ready = false
					break
				}
				level = max(level, w+1)
			}
			if ready {
				wave[id], progressed = level, true
			}
		}
		if !progressed {
			var cyclic []string
			for id := range deps {
				if _, done := wave[id]; !done {
					cyclic = append(cyclic, id)
				}
			}
			sort.Strings(cyclic)
			return wave, cyclic
		}
	}
	return wave, nil
}

// citedExcerpts verifies each citation and extracts its region with a
// margin: the quote must be on the cited line, whitespace collapsed.
func (d *CeremonyDriver) citedExcerpts(ctx context.Context, task domain.PlanTask) ([]string, []string, error) {
	var excerpts, problems []string
	for _, cite := range task.Context {
		clean, ok := workspacePath(cite.Path)
		if !ok {
			problems = append(problems, fmt.Sprintf("task %s: cited path %q is outside the workspace", task.ID, cite.Path))
			continue
		}
		content, found, err := d.Files.Read(ctx, clean, maxCitedFile)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			problems = append(problems, fmt.Sprintf("task %s: cited file %s does not exist", task.ID, clean))
			continue
		}
		lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		if cite.Line < 1 || cite.Line > len(lines) {
			problems = append(problems, fmt.Sprintf("task %s: %s has %d lines; line %d is out of range", task.ID, clean, len(lines), cite.Line))
			continue
		}
		quote, actual := collapse(cite.Quote), collapse(lines[cite.Line-1])
		if len(quote) < minCitationQuote || !strings.Contains(actual, quote) {
			problems = append(problems, fmt.Sprintf("task %s: %s:%d does not contain the quote (at least %d characters); the line reads %q", task.ID, clean, cite.Line, minCitationQuote, bounded(actual, 80)))
			continue
		}
		first, last := max(1, cite.Line-citationMargin), min(len(lines), cite.Line+citationMargin)
		excerpts = append(excerpts, fmt.Sprintf("--- %s:%d-%d ---\n%s", clean, first, last, strings.Join(lines[first-1:last], "\n")))
	}
	return excerpts, problems, nil
}

// taskPack is the whole context a worker starts from: its task, the shared
// interfaces, its check, what it must not touch and the verified excerpts.
// Notes addressed to it are added when it starts.
func taskPack(slug string, task domain.PlanTask, interfaces string, excerpts []string) string {
	var pack strings.Builder
	fmt.Fprintf(&pack, "Task %s of plan %s, wave %d: %s\n", task.ID, slug, task.Wave, task.Goal)
	scope := make([]string, 0, len(task.Scope))
	for _, p := range task.Scope {
		label := p
		for _, n := range task.New {
			if n == p {
				label += " (new)"
			}
		}
		scope = append(scope, label)
	}
	fmt.Fprintf(&pack, "Scope (the only files you may change): %s\n", strings.Join(scope, ", "))
	fmt.Fprintf(&pack, "Unit check: %s\n", strings.TrimSpace(task.UnitCheck.Program+" "+strings.Join(task.UnitCheck.Args, " ")))
	if task.TestFirst {
		pack.WriteString("Test first: write the failing test before the change.\n")
	}
	if len(task.Protect) > 0 {
		fmt.Fprintf(&pack, "Do not change: %s\n", strings.Join(task.Protect, ", "))
	}
	if strings.TrimSpace(interfaces) != "" {
		fmt.Fprintf(&pack, "Interfaces shared by the plan:\n%s\n", strings.TrimSpace(interfaces))
	}
	for _, excerpt := range excerpts {
		pack.WriteString(excerpt + "\n")
	}
	return pack.String()
}

// planSlug names a plan from its brief: a few words and a random suffix.
func planSlug(brief string) string {
	words := strings.FieldsFunc(strings.ToLower(brief), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	if len(words) > 3 {
		words = words[:3]
	}
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	slug := strings.Join(append(words, hex.EncodeToString(suffix[:])), "-")
	if len(slug) > 40 {
		slug = slug[len(slug)-40:]
		slug = strings.TrimLeft(slug, "-")
	}
	return slug
}

// decompose handles the planner's hand-back: verify, then record the plan
// and go to approval, or return the exact defects for the next round.
func (d *CeremonyDriver) decompose(ctx context.Context, s domain.Session, run domain.CeremonyRun, done stepDone, report map[string]any) (map[string]any, string, bool, string, error) {
	if d.Plans == nil || d.Files == nil {
		return nil, "", false, "plans need the workspace files and the plan registry", nil
	}
	record, err := d.planRecord(ctx, run.Plan.ID)
	if err != nil {
		return nil, "", false, "", err
	}
	v, err := d.verifyPlan(ctx, run.Plan.ID, done)
	if err != nil {
		return nil, "", false, "", err
	}
	record.Defects, record.Updated = v.defects, d.now()
	if len(v.defects) > 0 {
		run.Plan.Defects = v.defects
		output := map[string]any{"verified": false, "settled": false, "defects": v.defects}
		report["defects"] = v.defects
		if err := d.Plans.Save(ctx, record); err != nil {
			return nil, "", false, "", err
		}
		if run.Iteration >= ceremonyRepeatLimit {
			return output, "decompose_exhausted", false, "", nil
		}
		report["feedback"] = "the plan has defects; fix every one and call axlr_step_done again with the whole plan"
		return output, "", true, "", nil
	}
	record.Tasks, record.E2E, record.E2EBaseline, record.Waves = v.tasks, v.e2e, v.baseline, v.waves
	record.Interfaces, record.SummaryEN, record.Status = done.Interfaces, bounded(done.SummaryEN, 1500), domain.PlanAwaitingApproval
	if err := d.Plans.Save(ctx, record); err != nil {
		return nil, "", false, "", err
	}
	run.Plan.Defects = nil
	summary := make([]map[string]any, 0, len(v.tasks))
	for _, t := range v.tasks {
		summary = append(summary, map[string]any{"id": t.ID, "goal": t.Goal, "wave": t.Wave, "scope": t.Scope, "unit_check": t.UnitCheck.Program + " " + strings.Join(t.UnitCheck.Args, " "), "baseline": t.Baseline, "test_first": t.TestFirst, "depends_on": t.DependsOn})
	}
	output := map[string]any{"verified": true, "settled": true, "waves": v.waves, "tasks": summary, "e2e_check": v.e2e.Program + " " + strings.Join(v.e2e.Args, " "), "e2e_baseline": v.baseline, "summary_en": record.SummaryEN}
	report["tasks"], report["waves"] = len(v.tasks), v.waves
	return output, "decomposed", false, "", nil
}

func (d *CeremonyDriver) planRecord(ctx context.Context, id string) (domain.PlanRecord, error) {
	records, err := d.Plans.Load(ctx)
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

// setPlanStatus records a plan's status and, optionally, the person's
// decision; registry failures are reported, not fatal to MADE's record.
func (d *CeremonyDriver) setPlanStatus(ctx context.Context, id string, status domain.PlanStatus, decision, reason string) error {
	if d.Plans == nil {
		return nil
	}
	record, err := d.planRecord(ctx, id)
	if err != nil {
		return err
	}
	record.Status, record.Updated = status, d.now()
	if decision != "" {
		record.Decision, record.Reason = decision, reason
	}
	return d.Plans.Save(ctx, record)
}

// approvePlan is the person's a on a plan: complete present, grant the guard
// as the approver and apply approved. Each landed part is kept.
func (d *CeremonyDriver) approvePlan(ctx context.Context, s domain.Session, run domain.CeremonyRun) (StepResult, error) {
	p := run.Plan
	keep := func(err error) (StepResult, error) { return StepResult{Run: &run}, err }
	if p.Decided != "" && p.Decided != "approve" {
		return StepResult{}, fmt.Errorf("MADE already recorded %s for this plan", p.Decided)
	}
	if p.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "approve"})
		if err != nil {
			return StepResult{}, err
		}
		p.Decided = decided
		if decided != "approve" {
			return keep(fmt.Errorf("MADE already recorded %s for this plan", decided))
		}
	}
	if !p.Granted {
		if err := d.Approver.ApproveGuard(ctx, run.Instance, "person_approves"); err != nil {
			return keep(fmt.Errorf("approve as the person: %w", err))
		}
		p.Granted = true
	}
	before := copyRun(run)
	p.Awaiting = ""
	if err := d.setPlanStatus(ctx, p.ID, domain.PlanAwaitingApproval, "approve", ""); err != nil {
		return keep(err)
	}
	return d.leaveApproval(ctx, s, run, before, "approved", map[string]any{"step": run.Step, "decision": "approve"})
}

// returnPlan is the person's d on a plan: back to decompose with the reason.
func (d *CeremonyDriver) returnPlan(ctx context.Context, s domain.Session, run domain.CeremonyRun, reason string) (StepResult, error) {
	p := run.Plan
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return StepResult{}, errors.New("say why the plan goes back")
	case p.Decided != "" && p.Decided != "return":
		return StepResult{}, fmt.Errorf("MADE already recorded %s for this plan", p.Decided)
	case p.Returns >= domain.MaxPlanReturns && p.Decided == "":
		return StepResult{}, fmt.Errorf("the plan has gone back %d times; approve or decline it", p.Returns)
	}
	reason = bounded(reason, 2000)
	if p.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "return", "reason": reason})
		if err != nil {
			return StepResult{}, err
		}
		if decided != "return" {
			p.Decided = decided
			return StepResult{Run: &run}, fmt.Errorf("MADE already recorded %s for this plan", decided)
		}
		p.Decided, p.Returns, p.ReturnReason = "return", p.Returns+1, reason
	}
	before := copyRun(run)
	p.Awaiting, p.Decided = "", ""
	if err := d.setPlanStatus(ctx, p.ID, domain.PlanDecomposing, "return", reason); err != nil {
		return StepResult{Run: &before}, err
	}
	return d.leaveApproval(ctx, s, run, before, "returned", map[string]any{"step": run.Step, "decision": "return", "reason": reason})
}

// DeclinePlan is the person's x on a plan: the plan ends BLOCKED.
func (d *CeremonyDriver) DeclinePlan(ctx context.Context, s domain.Session, reason string) (StepResult, error) {
	run, _, err := d.awaiting(s)
	if err != nil {
		return StepResult{}, err
	}
	p := run.Plan
	if p == nil {
		return StepResult{}, errors.New("only a plan can be declined here")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return StepResult{}, errors.New("say why the plan is declined")
	}
	reason = bounded(reason, 2000)
	if p.Decided != "" && p.Decided != "decline" {
		return StepResult{}, fmt.Errorf("MADE already recorded %s for this plan", p.Decided)
	}
	if p.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "decline", "reason": reason})
		if err != nil {
			return StepResult{}, err
		}
		if p.Decided = decided; decided != "decline" {
			return StepResult{Run: &run}, fmt.Errorf("MADE already recorded %s for this plan", decided)
		}
	}
	before := copyRun(run)
	p.Awaiting = ""
	if err := d.setPlanStatus(ctx, p.ID, domain.PlanDeclined, "decline", reason); err != nil {
		return StepResult{Run: &before}, err
	}
	return d.leaveApproval(ctx, s, run, before, "declined", map[string]any{"step": run.Step, "decision": "decline", "reason": reason})
}

// awaitPlan claims present for the person, or records the automatic
// approval when settings ask for it.
func (d *CeremonyDriver) awaitPlan(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	if err := d.claimFor(ctx, &run, presentLease); err != nil {
		return StepResult{}, err
	}
	p := run.Plan
	p.Awaiting, p.Decided, p.Granted = domain.AwaitingApproval, "", false
	if d.Plan.AutoApprove {
		if _, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "automatic"}); err != nil {
			return StepResult{}, err
		}
		before := copyRun(run)
		p.Awaiting, p.Decided = "", "automatic"
		if err := d.setPlanStatus(ctx, p.ID, domain.PlanAwaitingApproval, "automatic", ""); err != nil {
			return StepResult{Run: &before}, err
		}
		return d.leaveApproval(ctx, s, run, before, "approved_automatically", report)
	}
	report["next_step"], report["instruction"] = run.Step, "The plan is verified and waits for the person's decision on the plan card (/plan). Tell the user in their language and end your turn."
	return accept(report, &run), nil
}

// finishPlan records the plan's terminal state in the registry and memory.
func (d *CeremonyDriver) finishPlan(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string) string {
	status := domain.PlanBlocked
	switch {
	case state == "READY":
		status = domain.PlanReady
	case run.Plan.Decided == "decline":
		status = domain.PlanDeclined
	}
	record, err := d.planRecord(ctx, run.Plan.ID)
	if err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	record.Status, record.Instance, record.Updated = status, run.Instance, d.now()
	if record.Decision == "" {
		record.Decision = run.Plan.Decided
	}
	if err := d.Plans.Save(ctx, record); err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	if d.Memory == nil || status != domain.PlanReady {
		return d.record(ctx, s, run, state, map[string]any{"summary_en": record.SummaryEN})
	}
	var table strings.Builder
	for _, t := range record.Tasks {
		fmt.Fprintf(&table, "%s (wave %d): %s; scope %s; check %s %s\n", t.ID, t.Wave, t.Goal, strings.Join(t.Scope, ", "), t.UnitCheck.Program, strings.Join(t.UnitCheck.Args, " "))
	}
	labels := map[string][]string{"ceremony": {run.Definition}, "plan": {record.ID}, "session": {string(s.Export().ID)}, "ws": {string(s.Export().Workspace)}}
	summary := fmt.Sprintf("Plan %s approved (%s): %d tasks in %d waves. %s", record.ID, record.Decision, len(record.Tasks), record.Waves, record.SummaryEN)
	evidence := fmt.Sprintf("MADE instance %s; e2e %s %s (baseline exit %d)\n%s", run.Instance, record.E2E.Program, strings.Join(record.E2E.Args, " "), record.E2EBaseline, bounded(table.String(), 4000))
	if _, err := d.Memory.RecordLinked(ctx, run.About, labels, MemoryRecord{ID: run.Instance + "-plan", Kind: "decision", Summary: summary, Evidence: evidence}); err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	return "recorded in " + run.About
}

// planInstruction adds what the planner must fix: the person's reason for a
// return and the defects of the last unverified round.
func planInstruction(run domain.CeremonyRun) string {
	p := run.Plan
	if p == nil || run.Step != "decompose" {
		return ""
	}
	text := ""
	if p.ReturnReason != "" {
		text += " The person returned the plan: " + p.ReturnReason + "."
	}
	if len(p.Defects) > 0 {
		text += " Defects of the last proposal: " + strings.Join(p.Defects, "; ") + "."
	}
	return text
}

// PlanProposal returns the plan awaiting the person, for the plan card.
func (d *CeremonyDriver) PlanProposal(ctx context.Context, s domain.Session) (domain.PlanRecord, error) {
	run, live := s.Ceremony()
	if !live || run.Plan == nil || d == nil || d.Plans == nil {
		return domain.PlanRecord{}, errors.New("no plan is waiting for approval")
	}
	return d.planRecord(ctx, run.Plan.ID)
}
