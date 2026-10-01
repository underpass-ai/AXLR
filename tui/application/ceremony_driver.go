package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ceremonyRepeatLimit matches max_iterations of the repeating steps in the
// 2.0 definitions.
const ceremonyRepeatLimit = 3

const maxCeremonyMemoryBytes = 2 << 10

type ceremonySpec struct {
	definition, version, first, briefInput string
}

var ceremonySpecs = map[domain.WorkMode]ceremonySpec{
	domain.ModeDebug:    {definition: "axlr_debug", version: "2.0", first: "reproduce", briefInput: "failure_brief"},
	domain.ModeDelivery: {definition: "axlr_delivery", version: "2.0", first: "brief", briefInput: "task_brief"},
}

var stateSteps = map[string]string{
	"REPRODUCE": "reproduce", "DIAGNOSE": "diagnose", "REPAIR": "repair",
	"BRIEF": "brief", "BUILD": "build", "INTEGRATE": "integrate",
}

// stepInstructions is what the model is asked to do in each step and what
// it must hand back through axlr_step_done.
var stepInstructions = map[string]string{
	"reproduce": "Find a command that shows the reported failure in this workspace. Call axlr_step_done with check_command {program, args} (no shell), expected and observed. The console runs the command: it must exit non-zero to count as reproduced. If no command can show the failure, call axlr_step_done with reproducible=false and explain why in observed.",
	"diagnose":  "Identify the first causal breach with the smallest discriminating probes; separate observation from inference. Do not edit yet. Call axlr_step_done with root_cause, evidence and proposed_fix.",
	"repair":    "Apply the smallest fix for the diagnosed cause and add a regression test when it protects real behaviour. Call axlr_step_done with summary. The console reruns the approved check command; it must exit zero.",
	"brief":     "Read the repository and settle the change. Call axlr_step_done with criteria (observable behaviour), scope and check_command {program, args} (no shell) whose zero exit proves the criteria. The console runs it once as a baseline.",
	"build":     "Implement the smallest change that meets the criteria; in later rounds fix what the previous check output shows, without growing scope. Call axlr_step_done with summary. The console reruns the approved check command; it must exit zero.",
	"integrate": "Write the report for the user: what changed, the evidence and the limits. Call axlr_step_done with report. The console records the revision.",
}

// CeremonyDriver walks a MADE ceremony on the model's behalf: it starts the
// instance, claims each step, checks the model's result with commands it
// runs itself, completes the step and applies the transition.
type CeremonyDriver struct {
	Engine CeremonyEnginePort
	Checks CheckRunnerPort
	Memory MemoryPort
	Now    func() time.Time
}

// StepResult is what one axlr_step_done call did. Run is nil when the
// ceremony reached a terminal state.
type StepResult struct {
	Outcome  domain.ToolOutcome
	Run      *domain.CeremonyRun
	Accepted bool
}

type stepDone struct {
	CheckCommand *struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	} `json:"check_command"`
	Reproducible *bool  `json:"reproducible"`
	Expected     string `json:"expected"`
	Observed     string `json:"observed"`
	RootCause    string `json:"root_cause"`
	Evidence     string `json:"evidence"`
	ProposedFix  string `json:"proposed_fix"`
	Criteria     string `json:"criteria"`
	Scope        string `json:"scope"`
	Summary      string `json:"summary"`
	Report       string `json:"report"`
}

func decodeStepDone(arguments root.JSONValue) (stepDone, error) {
	var done stepDone
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&done); err != nil {
		return stepDone{}, fmt.Errorf("axlr_step_done arguments: %w", err)
	}
	return done, nil
}

func (d stepDone) command() (domain.CheckCommand, bool) {
	if d.CheckCommand == nil || strings.TrimSpace(d.CheckCommand.Program) == "" {
		return domain.CheckCommand{}, false
	}
	return domain.CheckCommand{Program: d.CheckCommand.Program, Args: append([]string(nil), d.CheckCommand.Args...)}, true
}

// stepDoneNeedsApproval is true when the call proposes a check command the
// user has not approved for this ceremony: the approval card then shows it.
func stepDoneNeedsApproval(s domain.Session, arguments root.JSONValue) bool {
	done, err := decodeStepDone(arguments)
	if err != nil {
		return false // malformed calls are refused by the driver, not executed
	}
	proposed, ok := done.command()
	if !ok {
		return false
	}
	run, live := s.Ceremony()
	return !live || !proposed.Equal(run.Check)
}

// Begin starts the ceremony the session's mode names, before the turn that
// carries the user's request.
func (d *CeremonyDriver) Begin(ctx context.Context, s *domain.Session, prompt root.Text) error {
	spec, ok := ceremonySpecs[s.Mode()]
	if !ok {
		return errors.New("this mode starts no ceremony")
	}
	if d == nil || d.Engine == nil || d.Checks == nil {
		return errors.New("MADE is not connected; ceremonies are unavailable")
	}
	if err := d.Engine.Ready(ctx, spec.definition, spec.version); err != nil {
		return err
	}
	state := s.Export()
	about := "ws:" + string(state.ID)
	memory := ""
	if d.Memory != nil {
		if text, err := d.Memory.Wake(ctx, about); err == nil {
			memory = bounded(text, maxCeremonyMemoryBytes)
		}
	}
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	instance := fmt.Sprintf("axlr-%s-%d", state.ID, now().UTC().Unix())
	inputs := map[string]string{spec.briefInput: string(prompt), "workspace": string(state.Workspace), "memory_about": about}
	if err := d.Engine.Start(ctx, spec.definition, spec.version, instance, inputs); err != nil {
		return fmt.Errorf("start %s %s: %w", spec.definition, spec.version, err)
	}
	run := domain.CeremonyRun{Definition: spec.definition, Version: spec.version, Instance: instance, Step: spec.first, Iteration: 1, About: about, Memory: memory}
	fence, err := d.Engine.Claim(ctx, instance, run.Step, claimKey(run))
	if err != nil {
		return fmt.Errorf("claim %s: %w", run.Step, err)
	}
	run.Fence = fence
	return s.SetCeremony(run)
}

func claimKey(run domain.CeremonyRun) string {
	return fmt.Sprintf("%s:%s:%d", run.Instance, run.Step, run.Iteration)
}

// Instruction is the guidance line for the session's current step.
func Instruction(run domain.CeremonyRun) string {
	attempt := ""
	if run.Step == "reproduce" || run.Step == "repair" || run.Step == "build" {
		attempt = fmt.Sprintf(" (attempt %d of %d)", run.Iteration, ceremonyRepeatLimit)
	}
	text := fmt.Sprintf("Ceremony %s %s is running; the console drives MADE, so never call made_* tools for it. Current step: %s%s. %s", run.Definition, run.Version, run.Step, attempt, stepInstructions[run.Step])
	if !run.Check.IsZero() {
		text += fmt.Sprintf(" Approved check command: %s.", run.Check.Program+" "+strings.Join(run.Check.Args, " "))
	}
	if run.Memory != "" {
		text += " Project memory for this session (" + run.About + "): " + run.Memory
	}
	return text + "\n"
}

// StepDone checks the model's result for the current step, completes it in
// MADE and advances. Refusals leave MADE untouched so the model can retry.
func (d *CeremonyDriver) StepDone(ctx context.Context, s domain.Session, arguments root.JSONValue) (StepResult, error) {
	run, live := s.Ceremony()
	if !live {
		return refuse("no ceremony is running in this session"), nil
	}
	if d == nil || d.Engine == nil || d.Checks == nil {
		return refuse("MADE is not connected; the ceremony cannot advance"), nil
	}
	done, err := decodeStepDone(arguments)
	if err != nil {
		return refuse(err.Error()), nil
	}
	report := map[string]any{"step": run.Step, "iteration": run.Iteration}
	output := map[string]any{}
	var trigger string
	repeat := false
	switch run.Step {
	case "reproduce":
		if done.Reproducible != nil && !*done.Reproducible {
			output = map[string]any{"reproducible": false, "reproduced": false, "settled": true, "observed": done.Observed}
			trigger = "not_reproducible"
			break
		}
		command, ok := done.command()
		if !ok || done.Expected == "" || done.Observed == "" {
			return refuse("reproduce needs check_command {program, args}, expected and observed, or reproducible=false"), nil
		}
		run.Check = command
		result, err := d.Checks.Run(ctx, command)
		if err != nil {
			return StepResult{}, err
		}
		reproduced := result.ExitCode != 0
		output = map[string]any{"expected": done.Expected, "observed": done.Observed, "reproduced": reproduced, "settled": reproduced}
		addEvidence(output, report, command, result)
		switch {
		case reproduced:
			trigger = "reproduced"
		case run.Iteration >= ceremonyRepeatLimit:
			trigger = "reproduce_exhausted"
		default:
			repeat = true
			report["feedback"] = "the command exited 0, so it does not show the failure; find a command that fails"
		}
	case "diagnose":
		if done.RootCause == "" || done.Evidence == "" || done.ProposedFix == "" {
			return refuse("diagnose needs root_cause, evidence and proposed_fix"), nil
		}
		output = map[string]any{"root_cause": done.RootCause, "evidence": done.Evidence, "proposed_fix": done.ProposedFix, "diagnosed": true}
		trigger = "diagnosed"
	case "brief":
		command, ok := done.command()
		if !ok || done.Criteria == "" || done.Scope == "" {
			return refuse("brief needs criteria, scope and check_command {program, args}"), nil
		}
		run.Check = command
		result, err := d.Checks.Run(ctx, command)
		if err != nil {
			return StepResult{}, err
		}
		output = map[string]any{"criteria": done.Criteria, "scope": done.Scope, "ready": true}
		addEvidence(output, report, command, result)
		trigger = "briefed"
	case "repair", "build":
		if done.Summary == "" {
			return refuse(run.Step + " needs summary"), nil
		}
		if command, ok := done.command(); ok {
			run.Check = command
		}
		if run.Check.IsZero() {
			return refuse("no approved check command; propose one in check_command"), nil
		}
		result, err := d.Checks.Run(ctx, run.Check)
		if err != nil {
			return StepResult{}, err
		}
		passed := result.ExitCode == 0
		field, success, exhausted := "repaired", "repaired", "repair_exhausted"
		if run.Step == "build" {
			field, success, exhausted = "verified", "verified", "build_exhausted"
		}
		output = map[string]any{"summary": done.Summary, field: passed}
		addEvidence(output, report, run.Check, result)
		switch {
		case passed:
			trigger = success
		case run.Iteration >= ceremonyRepeatLimit:
			trigger = exhausted
		default:
			repeat = true
			report["feedback"] = "the check command still fails; fix what its output shows"
		}
	case "integrate":
		if done.Report == "" {
			return refuse("integrate needs report"), nil
		}
		revision, _ := d.Checks.Run(ctx, domain.CheckCommand{Program: "git", Args: []string{"rev-parse", "HEAD"}})
		dirty, _ := d.Checks.Run(ctx, domain.CheckCommand{Program: "git", Args: []string{"status", "--porcelain"}})
		output = map[string]any{"report": done.Report, "revision": strings.TrimSpace(revision.Output), "dirty": strings.TrimSpace(dirty.Output), "integrated": true}
		report["revision"] = output["revision"]
		trigger = "integrated"
	default:
		return refuse("unknown ceremony step " + run.Step), nil
	}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return StepResult{}, fmt.Errorf("complete %s: %w", run.Step, err)
	}
	if repeat {
		run.Iteration++
		if err := d.claim(ctx, &run); err != nil {
			return StepResult{}, err
		}
		report["next_step"], report["instruction"] = run.Step, stepInstructions[run.Step]
		return accept(report, &run), nil
	}
	state, err := d.Engine.Transition(ctx, run.Instance, trigger)
	if err != nil {
		return StepResult{}, fmt.Errorf("transition %s: %w", trigger, err)
	}
	report["state"] = state
	if state == "COMPLETED" || state == "BLOCKED" {
		d.record(ctx, s, run, state, output)
		report["ceremony"] = state
		report["instruction"] = "The ceremony is over and the session is back in normal mode. Tell the user the outcome in their language."
		return accept(report, nil), nil
	}
	next, ok := stateSteps[state]
	if !ok {
		return StepResult{}, fmt.Errorf("ceremony entered unknown state %s", state)
	}
	run.Step, run.Iteration = next, 1
	if err := d.claim(ctx, &run); err != nil {
		return StepResult{}, err
	}
	report["next_step"], report["instruction"] = run.Step, stepInstructions[run.Step]
	return accept(report, &run), nil
}

func (d *CeremonyDriver) claim(ctx context.Context, run *domain.CeremonyRun) error {
	fence, err := d.Engine.Claim(ctx, run.Instance, run.Step, claimKey(*run))
	if err != nil {
		return fmt.Errorf("claim %s: %w", run.Step, err)
	}
	run.Fence = fence
	return nil
}

// record writes the outcome to the session's KMP about. Memory is best effort:
// MADE already holds the durable record.
func (d *CeremonyDriver) record(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string, output map[string]any) {
	if d.Memory == nil {
		return
	}
	summary := fmt.Sprintf("%s %s ended %s at step %s.", run.Definition, run.Version, state, run.Step)
	if text, ok := output["report"].(string); ok && text != "" {
		summary += " " + bounded(text, 1500)
	}
	evidence := fmt.Sprintf("MADE instance %s", run.Instance)
	if !run.Check.IsZero() {
		evidence += "; check " + run.Check.Program + " " + strings.Join(run.Check.Args, " ")
	}
	if revision, ok := output["revision"].(string); ok && revision != "" {
		evidence += "; revision " + revision
	}
	labels := map[string][]string{"ceremony": {run.Definition}, "step": {run.Step}, "ws": {string(s.Export().Workspace)}}
	_ = d.Memory.Record(ctx, run.About, labels, run.Instance, summary, evidence)
}

func addEvidence(output, report map[string]any, command domain.CheckCommand, result CheckResult) {
	evidence := map[string]any{"program": command.Program, "args": command.Args, "exit_code": result.ExitCode, "output_tail": result.Output}
	output["check"] = evidence
	report["check"] = evidence
}

func refuse(reason string) StepResult {
	encoded, _ := json.Marshal(map[string]any{"accepted": false, "error": reason})
	return StepResult{Outcome: domain.ToolOutcome{Content: root.Text(encoded), IsError: true}}
}

func accept(report map[string]any, run *domain.CeremonyRun) StepResult {
	report["accepted"] = true
	encoded, _ := json.Marshal(report)
	return StepResult{Outcome: domain.ToolOutcome{Content: root.Text(encoded)}, Run: run, Accepted: true}
}

func bounded(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8Start(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
