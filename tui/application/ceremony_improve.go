package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The improve ceremony is the repair ceremony's sibling for changes that are
// not failures: brief and build are delivery's steps under an improvement's
// contract, and propose, watch, decide and merge are repair's console steps.

// improveInstructions replace the delivery wording of brief and build: the
// baseline must fail and build also returns summary_en.
var improveInstructions = map[string]string{
	"brief": "Read the clone and settle the improvement. Call axlr_step_done with criteria (observable behaviour), scope and check_command {program, args} (no shell) whose zero exit proves the criteria. The console runs it once before any change: it must exit non-zero, showing the improvement is missing, and the same command must pass after build. If the improvement already exists, or cannot be made safely as a small change, call axlr_step_done with feasible=false and explain why in observed; the console then closes the ceremony as BLOCKED.",
	"build": "Implement the smallest change that meets the criteria and add a test when it protects real behaviour; in later rounds fix what the previous check output shows, without growing scope. Call axlr_step_done with summary and summary_en, two or three plain English sentences for the pull request and project memory. The console reruns the check command approved in brief, which is fixed for the rest of the ceremony; it must exit zero.",
}

// improveInstruction is repairInstruction for an improvement.
func improveInstruction(run domain.CeremonyRun) string {
	r := run.Repair
	var text strings.Builder
	fmt.Fprintf(&text, " Improvement of %s in this clone (branch %s).", r.Repository, r.Branch)
	if run.Step == "build" {
		text.WriteString(" Do not commit, push or open pull requests yourself: the console does that once the check passes, and the person decides the merge.")
		if r.Rounds > 0 && r.Feedback != "" {
			fmt.Fprintf(&text, " Check round %d of %d came back red on pull request #%d; failing checks: %s", r.Rounds, domain.MaxRepairRounds, r.PullRequest, r.Feedback)
		}
	}
	return text.String()
}

// improveBrief is brief for an improvement: the check must fail before any
// change, so the pull request can show it failing before and passing after.
// feasible=false closes the ceremony with the reason instead.
func (d *CeremonyDriver) improveBrief(ctx context.Context, run *domain.CeremonyRun, done stepDone, report map[string]any) (output map[string]any, trigger string, repeat bool, refusal string, err error) {
	if done.Feasible != nil && !*done.Feasible {
		if strings.TrimSpace(done.Observed) == "" {
			return nil, "", false, "feasible=false needs observed: why the improvement already exists or cannot be made safely", nil
		}
		return map[string]any{"feasible": false, "missing": false, "settled": true, "observed": done.Observed}, "not_feasible", false, "", nil
	}
	command, ok := done.command()
	if !ok || done.Criteria == "" || done.Scope == "" {
		return nil, "", false, "brief needs criteria, scope and check_command {program, args}, or feasible=false with observed", nil
	}
	run.Check = command
	result, err := d.Checks.Run(ctx, command)
	if err != nil {
		return nil, "", false, "", err
	}
	if !result.Ran {
		return nil, "", false, "the check command did not run (" + result.Output + "); give program and args separately, with a program that exists", nil
	}
	missing := result.ExitCode != 0
	output = map[string]any{"criteria": done.Criteria, "scope": done.Scope, "missing": missing, "settled": missing}
	addEvidence(output, report, command, result)
	switch {
	case missing:
		run.Repair.Criteria, run.Repair.Scope = bounded(done.Criteria, 1500), bounded(done.Scope, 1500)
		return output, "briefed", false, "", nil
	case run.Iteration >= ceremonyRepeatLimit:
		return output, "brief_exhausted", false, "", nil
	}
	report["feedback"] = "the check command exits 0 before any change, so it does not show that the improvement is missing; find a check that fails until the criteria are met"
	return output, "", true, "", nil
}

// improveBody is repairBody for an improvement: the criteria take the place
// of the cause, and the evidence is the check that failed before the change.
func improveBody(run domain.CeremonyRun) string {
	r := run.Repair
	var body strings.Builder
	fmt.Fprintf(&body, "Improvement made by the AXLR console through MADE ceremony `%s` %s, instance `%s`. It merges only with the person's approval.\n\n", run.Definition, run.Version, run.Instance)
	if r.Criteria != "" {
		body.WriteString("## Criteria\n\n" + r.Criteria + "\n\n")
	}
	if r.Scope != "" {
		body.WriteString("## Scope\n\n" + r.Scope + "\n\n")
	}
	if r.Summary != "" {
		body.WriteString("## Summary\n\n" + r.Summary + "\n\n")
	}
	if !run.Check.IsZero() {
		fmt.Fprintf(&body, "## Evidence\n\nCheck command `%s %s` failed before the change and passed after it, run by the console.\n", run.Check.Program, strings.Join(run.Check.Args, " "))
	}
	if r.Rounds > 0 {
		fmt.Fprintf(&body, "\nCheck round %d after a red round: %s\n", r.Rounds+1, bounded(r.Feedback, 1000))
	}
	return body.String()
}
