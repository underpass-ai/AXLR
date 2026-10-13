package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /ceremony <definition> [version] [brief] drives any published definition
// with the generic loop (decisions H3 and H4 of the lifecycle integration
// design): the console starts the instance with the brief as every
// required input it does not know, claims the step MADE says is
// claimable, hands the model the step's own prompt from the definition,
// completes the step with the fields the model returns, applies the
// transition MADE enables and claims the next step, until a terminal
// state. The person publishes definitions; the model designs them.

// VersionLess orders versions numerically by dotted component ("2.0" after
// "1.10"), then lexically.
func VersionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, errX := strconv.Atoi(as[i])
		y, errY := strconv.Atoi(bs[i])
		if errX != nil || errY != nil {
			if as[i] != bs[i] {
				return as[i] < bs[i]
			}
			continue
		}
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}

// LatestVersion picks the newest version of a definition among the listed.
func LatestVersion(listed []CeremonyDefinitionSummary, name string) string {
	latest := ""
	for _, item := range listed {
		if item.Name == name && (latest == "" || VersionLess(latest, item.Version)) {
			latest = item.Version
		}
	}
	return latest
}

// genericInputs are the inputs the console fills itself.
var genericInputs = map[string]bool{"workspace": true, "memory_about": true}

// beginGeneric starts the chosen definition for the person's brief.
func (d *CeremonyDriver) beginGeneric(ctx context.Context, s *domain.Session, prompt root.Text, about, memory string, now func() time.Time) error {
	choice := s.ChosenCeremony()
	if choice.Definition == "" {
		return errors.New("no ceremony was chosen: /ceremony <definition> [version] [brief]")
	}
	if d.Definitions == nil {
		return errors.New("the orchestrator is not connected; ceremonies are unavailable")
	}
	definition, err := d.Definitions.Describe(ctx, choice.Definition, choice.Version)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", choice.Definition, choice.Version, err)
	}
	state := s.Export()
	instance := fmt.Sprintf("axlr-%s-%d", state.ID, now().UTC().Unix())
	inputs := map[string]string{"workspace": string(state.Workspace), "memory_about": about}
	for _, input := range definition.Inputs {
		if !genericInputs[input] {
			inputs[input] = string(prompt)
		}
	}
	if err := d.Engine.Start(ctx, definition.Name, definition.Version, instance, inputs); err != nil {
		return fmt.Errorf("start %s %s: %w", definition.Name, definition.Version, err)
	}
	view, err := d.Engine.Inspect(ctx, instance)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", instance, err)
	}
	if len(view.Claimable) == 0 {
		return fmt.Errorf("%s %s starts with no claimable step in state %s", definition.Name, definition.Version, view.State)
	}
	run := domain.CeremonyRun{Definition: definition.Name, Version: definition.Version, Instance: instance, Step: view.Claimable[0], Iteration: 1, About: about, Memory: memory,
		Generic: &domain.GenericRun{Prompts: definition.Prompts, Terminal: definition.Terminal}}
	if err := d.claimFor(ctx, &run, 0); err != nil {
		return err
	}
	d.report(ctx, *s, &run, "running", "step")
	return s.SetCeremony(run)
}

// genericInstruction is a generic step's instruction: the definition's
// own prompt, and how to hand the result back.
func genericInstruction(run domain.CeremonyRun) string {
	prompt := ""
	if run.Generic != nil {
		prompt = run.Generic.Prompts[run.Step]
	}
	if prompt == "" {
		prompt = "Do the work of step " + run.Step + " of ceremony " + run.Definition + " as its definition describes (read the definition when needed)."
	}
	return prompt + " When the step's work is done, hand it back with axlr_step_done, giving the fields the step asks for as JSON fields."
}

// genericStepDone completes a generic step with the fields the model sent,
// applies the transition MADE enables and claims the next step.
func (d *CeremonyDriver) genericStepDone(ctx context.Context, s domain.Session, run domain.CeremonyRun, arguments root.JSONValue) (StepResult, error) {
	var output map[string]any
	if err := json.Unmarshal(arguments.Bytes(), &output); err != nil || output == nil {
		return refuse("axlr_step_done arguments must be a JSON object with the fields the step asks for"), nil
	}
	delete(output, WhyArgument)
	report := map[string]any{"step": run.Step, "iteration": run.Iteration}
	d.report(ctx, s, &run, "finished", "step")
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return refuse(fmt.Sprintf("complete %s: %v; correct the fields and hand the step back again", run.Step, bounded(err.Error(), 400))), nil
	}
	run.Fence = ""
	view, err := d.Engine.Inspect(ctx, run.Instance)
	if err != nil {
		return StepResult{}, fmt.Errorf("inspect %s: %w", run.Instance, err)
	}
	if len(view.Enabled) > 0 {
		trigger := view.Enabled[0]
		if len(view.Enabled) > 1 {
			report["other_triggers"] = view.Enabled[1:]
		}
		state, err := d.Engine.Transition(ctx, run.Instance, trigger)
		if err != nil {
			return StepResult{}, fmt.Errorf("transition %s: %w", trigger, err)
		}
		report["transition"] = trigger
		view, err = d.Engine.Inspect(ctx, run.Instance)
		if err != nil {
			return StepResult{}, fmt.Errorf("inspect %s: %w", run.Instance, err)
		}
		if view.State == "" {
			view.State = state
		}
	}
	report["state"] = view.State
	if run.Generic != nil && slices.Contains(run.Generic.Terminal, view.State) {
		report["memory"] = d.record(ctx, s, run, view.State, output)
		report["ceremony"] = view.State
		report["instruction"] = "The ceremony is over and the session is back in normal mode. Tell the user the outcome in their language."
		if view.State == "BLOCKED" {
			report["instruction"] = "The ceremony ended BLOCKED and the session is back in normal mode; its state stays with the orchestrator and its outcome in memory. Tell the user the outcome in their language and propose the next step: " + blockedNextSteps + "."
		}
		d.observe(run, view.State, report, true, false)
		return accept(report, nil), nil
	}
	if len(view.Claimable) == 0 {
		// Nothing to claim and no transition: the definition waits for
		// something the console does not drive (a human guard, a server
		// step). The run stays live for the person to decide.
		report["instruction"] = "The orchestrator enables no transition and offers no step to claim in state " + view.State + ": the ceremony waits for something outside this console (a guard, a server-owned step). Tell the user; they can wait, decide in MADE, or /stop-ceremony."
		d.observe(run, view.State, report, false, true)
		return accept(report, &run), nil
	}
	next := view.Claimable[0]
	if next == run.Step {
		run.Iteration++
	} else {
		run.Step, run.Iteration = next, 1
	}
	if err := d.claim(ctx, &run); err != nil {
		return StepResult{}, err
	}
	d.report(ctx, s, &run, "running", "step")
	report["next_step"], report["instruction"] = run.Step, genericInstruction(run)+stepProgress(run)
	d.observe(run, "", report, false, false)
	return accept(report, &run), nil
}
