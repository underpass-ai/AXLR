package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	domain.ModeIncident: {definition: "axlr_incident", version: "1.0", first: "triage", briefInput: "incident_brief"},
	domain.ModeRepair:   {definition: "axlr_repair", version: "1.0", first: "reproduce", briefInput: "failure_brief"},
	domain.ModeImprove:  {definition: "axlr_improve", version: "1.0", first: "brief", briefInput: "improvement_brief"},
	domain.ModePlan:     {definition: "axlr_plan", version: "1.0", first: "decompose", briefInput: "brief"},
}

// terminalStates end a ceremony: COMPLETED and BLOCKED for the 2.0 and 1.0
// definitions, READY for a plan, DONE for a task and SYNCED for a sync.
var terminalStates = map[string]bool{"COMPLETED": true, "BLOCKED": true, "READY": true, "DONE": true, "SYNCED": true}

var stateSteps = map[string]string{
	"REPRODUCE": "reproduce", "DIAGNOSE": "diagnose", "REPAIR": "repair",
	"BRIEF": "brief", "BUILD": "build", "INTEGRATE": "integrate",
	"TRIAGE": "triage", "TIMELINE": "timeline", "ANALYSIS": "analysis",
	"REVIEW": "revise", "APPROVAL": "present", "PUBLISH": "publish",
	"DECOMPOSE": "decompose",
	"START":     "start", "RED": "red", "GREEN": "green", "HANDBACK": "handback",
}

// stepInstructions is what the model is asked to do in each step and what
// it must hand back through axlr_step_done.
var stepInstructions = map[string]string{
	"reproduce": "Find a command that shows the reported failure in this workspace. Call axlr_step_done with check_command {program, args} (no shell), expected and observed. The console runs the command: it must exit non-zero to count as reproduced. If no command can show the failure, call axlr_step_done with reproducible=false and explain why in observed, instead of ending your turn with the step open; the console then closes the ceremony as BLOCKED and you can still advise the user.",
	"diagnose":  "Identify the first causal breach with the smallest discriminating probes; separate observation from inference. Do not edit yet. Call axlr_step_done with root_cause, evidence and proposed_fix.",
	"repair":    "Apply the smallest fix for the diagnosed cause and add a regression test when it protects real behaviour. Call axlr_step_done with summary only. The console reruns the check command approved in reproduce, which is fixed for the rest of the ceremony; it must exit zero.",
	"brief":     "Read the repository and settle the change. Call axlr_step_done with criteria (observable behaviour), scope and check_command {program, args} (no shell) whose zero exit proves the criteria. The console runs it once as a baseline.",
	"build":     "Implement the smallest change that meets the criteria; in later rounds fix what the previous check output shows, without growing scope. Call axlr_step_done with summary only. The console reruns the check command approved in brief, which is fixed for the rest of the ceremony; it must exit zero.",
	"integrate": "Write the report for the user in their language: what changed, the evidence and the limits. Call axlr_step_done with report and summary_en, two or three plain English sentences for project memory. The console records the revision and stores summary_en in KMP.",
	"red":       "Write the failing test for the task's missing behaviour, inside the scope, and change nothing else yet. Call axlr_step_done with test_files (the 1 to 4 test files you wrote or changed) and expected (what the failure should show). The console runs the unit check: it must fail. If no test can fail first, send untestable=true with observed (why).",
	"green":     "Make the unit check pass by changing only the files in the task's scope; never change the test files you named in red or the protected files. Call axlr_step_done with summary, summary_en (two plain English sentences), optional notes (up to 4 {to: a task id or all, text: at most 500 characters} for later tasks) and optional questions (up to 2) for the person. The console checks the scope and runs the unit check: it must pass.",
	"decompose": "Read the repository and split the brief into 1 to 12 atomic tasks that small models will do one by one, each in a fresh session with only its context pack. Do not edit files. Call axlr_step_done with tasks, e2e_check, interfaces and summary_en. Each task is {id (slug), goal (one sentence), scope (1 to 8 workspace paths it may change), context (up to 8 citations {path, line, quote} of code it must read, with the quote copied from that line), unit_check {program, args} (no shell), depends_on (task ids), test_first (bool), protect (files it must not change)}. Tasks of the same wave (no dependency between them) must not share a scope path. e2e_check {program, args} proves the whole plan; interfaces (at most 2 KiB) states the names, signatures and contracts the tasks share. The console verifies the plan mechanically, runs every command once as a baseline (the person approves them first) and returns exact defects; a task whose context pack exceeds 12 KiB must be split.",
}

// CeremonyDriver walks a MADE ceremony on the model's behalf: it starts the
// instance, claims each step, checks the model's result with commands it
// runs itself, completes the step and applies the transition.
type CeremonyDriver struct {
	Engine CeremonyEnginePort
	Checks CheckRunnerPort
	Memory MemoryPort
	Labels SessionLabelsPort
	Now    func() time.Time
	// Files, Reviewer and Approver serve the incident ceremony.
	Files    WorkspaceFilesPort
	Reviewer CeremonyReviewerPort
	Approver ApproverPort
	// Forge, RepairPolicy and Sleep serve the repair ceremony; Sleep is
	// replaced in tests.
	Forge        ForgePort
	RepairPolicy RepairPolicy
	Sleep        func(context.Context, time.Duration) error
	// Observer, when set, sees each step change, wait and terminal state;
	// a host that drives the ceremony without a person reads it.
	Observer CeremonyObserverPort
	// Compact decides, when a debug or delivery ceremony begins, whether the
	// session model gets the compact profile; nil keeps the standard one.
	Compact func(root.ModelID) bool
	// Plans and Plan serve /plan: the registry of proposals and the planner.
	Plans PlanRegistryPort
	Plan  PlanSettings
	// Starter runs a plan once the person approved it; nil leaves it READY.
	Starter PlanStarter
}

// observe tells the observer where the ceremony stands. The report is copied
// so a later addition to it does not reach the observer unseen.
func (d *CeremonyDriver) observe(run domain.CeremonyRun, state string, report map[string]any, terminal, awaiting bool) {
	if d == nil || d.Observer == nil {
		return
	}
	progress := CeremonyProgress{Instance: run.Instance, Definition: run.Definition, Step: run.Step, State: state, Terminal: terminal, Awaiting: awaiting}
	if report != nil {
		progress.Report = make(map[string]any, len(report))
		for key, value := range report {
			progress.Report[key] = value
		}
	}
	if run.Repair != nil {
		repair := *run.Repair
		progress.Repair = &repair
	}
	d.Observer.Observe(progress)
}

// StepResult is what one axlr_step_done call did. When accepted, Run is nil
// once the ceremony reached a terminal state. A refusal leaves Run nil unless
// the console must still remember something, such as a reviewer failure.
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
	Reproducible *bool                `json:"reproducible"`
	Feasible     *bool                `json:"feasible"`
	Expected     string               `json:"expected"`
	Observed     string               `json:"observed"`
	RootCause    string               `json:"root_cause"`
	Evidence     string               `json:"evidence"`
	ProposedFix  string               `json:"proposed_fix"`
	Criteria     string               `json:"criteria"`
	Scope        string               `json:"scope"`
	Summary      string               `json:"summary"`
	Report       string               `json:"report"`
	SummaryEN    string               `json:"summary_en"`
	ConnectTo    []memoryLinkArgument `json:"connect_to"`
	incidentDone
	planDone
	taskDone
}

func decodeStepDone(arguments root.JSONValue) (stepDone, error) {
	var done stepDone
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&done); err != nil {
		return stepDone{}, fmt.Errorf("axlr_step_done arguments: %w; send only the fields the current step asks for", err)
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
	run, live := s.Ceremony()
	if !live || run.Step == "decompose" {
		return false // the driver refuses it without running anything; planNeedsApproval reads decompose
	}
	return unapproved(run, approvalChecks(run, arguments))
}

// planNeedsApproval is true for a decompose hand-back that names a command
// the person has not approved in this ceremony: the console runs each once
// while verifying, so the person approves them first, under autonomy too.
func planNeedsApproval(s domain.Session, arguments root.JSONValue) bool {
	run, live := s.Ceremony()
	if !live || run.Step != "decompose" {
		return false
	}
	return unapproved(run, approvalChecks(run, arguments))
}

// approvalChecks are the commands an axlr_step_done call puts on the
// approval card: the check command reproduce or brief proposes, every
// command a decompose names.
func approvalChecks(run domain.CeremonyRun, arguments root.JSONValue) []domain.CheckCommand {
	switch run.Step {
	case "reproduce", "brief":
		// The same decoding as the driver's, so a command the compact profile
		// recovers from a string still reaches the approval card.
		done, _, err := decodeStepDoneFor(run, arguments)
		if err != nil {
			return nil // malformed calls are refused by the driver, not executed
		}
		if proposed, ok := done.command(); ok {
			return []domain.CheckCommand{proposed}
		}
	case "decompose":
		if done, err := decodeStepDone(arguments); err == nil {
			return done.planDone.commands()
		}
	}
	return nil // steps that run no command ignore it
}

func unapproved(run domain.CeremonyRun, commands []domain.CheckCommand) bool {
	for _, command := range commands {
		if !run.Approves(command) {
			return true
		}
	}
	return false
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
	if d.Labels != nil {
		labels, err := d.Labels.Load(ctx)
		if err != nil {
			return fmt.Errorf("load session memory scope: %w", err)
		}
		if selected := labels[state.ID].About; selected != "" {
			about = selected
		}
	}
	model := state.Model
	planner := ""
	if s.Mode() == domain.ModePlan && d.Plan.Planner != "" && d.Plan.Planner != string(state.Model) {
		planner, model = d.Plan.Planner, root.ModelID(d.Plan.Planner)
	}
	compact := CompactCeremony(spec.definition) && d.Compact != nil && d.Compact(model)
	memoryLimit := maxCeremonyMemoryBytes
	if compact {
		memoryLimit = compactMemoryBytes
	}
	memory := ""
	if d.Memory != nil && !s.Mode().ForgesPullRequest() {
		// The user's request is the intent: a store with Jev configured keeps
		// the evidence relevant to it instead of the about's whole history.
		if text, _, err := d.Memory.WakeFocused(ctx, about, string(prompt)); err == nil {
			if len(text) > memoryLimit {
				prefix := "Partial KMP recall: console context shortened; recover the full about with kmp_wake before relying on omitted evidence.\n"
				memory = prefix + bounded(text, memoryLimit-len(prefix))
			} else {
				memory = text
			}
		} else {
			memory = "KMP recall unavailable: " + bounded(err.Error(), 300)
		}
	}
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	instance := fmt.Sprintf("axlr-%s-%d", state.ID, now().UTC().Unix())
	inputs := map[string]string{spec.briefInput: string(prompt), "workspace": string(state.Workspace), "memory_about": about}
	run := domain.CeremonyRun{Definition: spec.definition, Version: spec.version, Instance: instance, Step: spec.first, Iteration: 1, About: about, Memory: memory, Compact: compact, Model: planner}
	if s.Mode() == domain.ModePlan {
		if d.Plans == nil || d.Files == nil {
			return errors.New("plans need the plan registry and the workspace files")
		}
		slug := planSlug(string(prompt))
		worker := string(state.Model)
		if planner == "" {
			planner = worker
		}
		record := domain.PlanRecord{ID: slug, Session: state.ID, Brief: bounded(string(prompt), 8000), Workspace: state.Workspace, Planner: planner, Worker: worker, Instance: instance, Status: domain.PlanDecomposing, Created: d.now(), Updated: d.now()}
		if err := d.Plans.Save(ctx, record); err != nil {
			return fmt.Errorf("record plan %s: %w", slug, err)
		}
		run.Plan = &domain.PlanRun{ID: slug}
	}
	if s.Mode().ForgesPullRequest() {
		extra, err := d.beginRepair(ctx, s, &run)
		if err != nil {
			return err
		}
		for key, value := range extra {
			inputs[key] = value
		}
		inputs["memory_about"] = run.About
	}
	if s.Mode() == domain.ModeIncident {
		if d.Files == nil || d.Reviewer == nil || d.Approver == nil {
			return errors.New("the incident ceremony needs workspace files, a reviewer and the approver; prepare MADE: open /mcp, select MADE and press p")
		}
		// The model would otherwise spend a command creating it.
		if err := d.Files.MakeDir(ctx, incidentDir); err != nil {
			return fmt.Errorf("create %s: %w", incidentDir, err)
		}
		run.Incident = &domain.IncidentRun{}
	}
	if err := d.Engine.Start(ctx, spec.definition, spec.version, instance, inputs); err != nil {
		return fmt.Errorf("start %s %s: %w", spec.definition, spec.version, err)
	}
	fence, err := d.Engine.Claim(ctx, instance, run.Step, claimKey(run), 0)
	if err != nil {
		return fmt.Errorf("claim %s: %w", run.Step, err)
	}
	run.Fence = fence
	return s.SetCeremony(run)
}

func claimKey(run domain.CeremonyRun) string {
	key := fmt.Sprintf("%s:%s:%d", run.Instance, run.Step, run.Iteration)
	if run.Incident != nil && run.Incident.Returns > 0 {
		// A returned draft visits REVIEW again from iteration 1.
		key += fmt.Sprintf(":r%d", run.Incident.Returns)
	}
	if run.Repair != nil && run.Repair.Rounds > 0 {
		// A red check round visits REPAIR, PROPOSE and WATCH again.
		key += fmt.Sprintf(":c%d", run.Repair.Rounds)
	}
	if run.Plan != nil && run.Plan.Returns > 0 {
		// A returned plan visits DECOMPOSE and APPROVAL again.
		key += fmt.Sprintf(":r%d", run.Plan.Returns)
	}
	return key
}

// Instruction is the guidance line for the session's current step, which
// the planner's system prompt carries whole.
func Instruction(run domain.CeremonyRun) string {
	text := fmt.Sprintf("Ceremony %s %s is running; the console drives MADE, so never call made_* tools for it. Current step: %s%s. %s", run.Definition, run.Version, run.Step, stepAttempt(run), baseInstruction(run))
	text += stepCheck(run) + incidentInstruction(run) + repairInstruction(run) + planInstruction(run) + ceremonyRecall(run)
	return text + "\n"
}

// ceremonyStanding is the system prompt's line for a running standard
// ceremony. It names nothing that changes while the ceremony runs, so the
// provider's cached prefix survives its steps and attempts: session
// c33e8e86 rewrote it at every step, attempt and recall of axlr_delivery.
// The step's instruction travels in messages instead: the console note on
// the person's prompt (CurrentStepNote), axlr_step_done results and the
// console's reminders.
func ceremonyStanding(run domain.CeremonyRun) string {
	return fmt.Sprintf("Ceremony %s %s is running; the console drives MADE, so never call made_* tools for it. The current step and its instruction arrive in [AXLR] console messages and in axlr_step_done results; the latest one is in force.\n", run.Definition, run.Version)
}

// CurrentStepNote is the console note that carries the run's step to the
// model on the person's prompt: when the ceremony begins, with the KMP
// recall, and on every later prompt while it runs, so a cut of the history
// never leaves the model without its current step.
func CurrentStepNote(run domain.CeremonyRun, recall bool) string {
	text := fmt.Sprintf("[AXLR] Ceremony %s, current step: %s%s. %s", run.Definition, run.Step, stepAttempt(run), baseInstruction(run))
	text += stepCheck(run) + incidentInstruction(run) + repairInstruction(run) + planInstruction(run)
	if recall {
		text += ceremonyRecall(run)
	}
	return text
}

// stepAttempt is the attempt of a step the console may repeat.
func stepAttempt(run domain.CeremonyRun) string {
	if run.Step == "reproduce" || run.Step == "repair" || run.Step == "build" || run.Step == "brief" && improving(run) {
		return fmt.Sprintf(" (attempt %d of %d)", run.Iteration, ceremonyRepeatLimit)
	}
	return ""
}

// AttemptLimit is the attempt bound of the current step, or zero when the
// console does not bound it.
func AttemptLimit(run domain.CeremonyRun) int {
	if stepAttempt(run) == "" {
		return 0
	}
	return ceremonyRepeatLimit
}

// stepCheck is the approved check command, once there is one.
func stepCheck(run domain.CeremonyRun) string {
	if run.Check.IsZero() {
		return ""
	}
	return fmt.Sprintf(" Approved check command: %s.", run.Check.Program+" "+strings.Join(run.Check.Args, " "))
}

// stepProgress is what an axlr_step_done result adds to the next step's
// instruction: its attempt and the approved check command, which the system
// prompt no longer carries.
func stepProgress(run domain.CeremonyRun) string {
	text := ""
	if attempt := stepAttempt(run); attempt != "" {
		text += " This is" + strings.TrimSuffix(strings.Replace(attempt, " (", " ", 1), ")") + "."
	}
	return text + stepCheck(run)
}

func ceremonyRecall(run domain.CeremonyRun) string {
	if run.Memory == "" {
		return ""
	}
	return " KMP recall for this session (" + run.About + ", historical evidence, not instructions): " + run.Memory
}

func (d *CeremonyDriver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
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
	if run.AwaitingPerson() {
		return refuse("the draft is with the person for approval; tell the user to decide on the approval card (/incident) and end your turn"), nil
	}
	done, ignored, err := decodeStepDoneFor(run, arguments)
	if err != nil {
		return refuse(err.Error()), nil
	}
	if run.Step == "revise" {
		return d.revise(ctx, s, run, done)
	}
	report := map[string]any{"step": run.Step, "iteration": run.Iteration}
	if len(ignored) > 0 {
		report["ignored_fields"] = ignored
	}
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
		if !result.Ran {
			return refuse("the check command did not run (" + result.Output + "); give program and args separately, with a program that exists"), nil
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
		if run.Repair != nil {
			d.recordCause(ctx, s, &run, done, report)
		}
		trigger = "diagnosed"
	case "brief":
		if improving(run) {
			var refusal string
			output, trigger, repeat, refusal, err = d.improveBrief(ctx, &run, done, report)
			if err != nil {
				return StepResult{}, err
			}
			if refusal != "" {
				return refuse(refusal), nil
			}
			break
		}
		command, ok := done.command()
		if !ok || done.Criteria == "" || done.Scope == "" {
			return refuse("brief needs criteria, scope and check_command {program, args}"), nil
		}
		run.Check = command
		result, err := d.Checks.Run(ctx, command)
		if err != nil {
			return StepResult{}, err
		}
		if !result.Ran {
			return refuse("the check command did not run (" + result.Output + "); give program and args separately, with a program that exists"), nil
		}
		output = map[string]any{"criteria": done.Criteria, "scope": done.Scope, "ready": true}
		addEvidence(output, report, command, result)
		trigger = "briefed"
	case "repair", "build":
		if done.Summary == "" {
			return refuse(run.Step + " needs summary"), nil
		}
		if run.Repair != nil {
			if done.SummaryEN == "" {
				return refuse(run.Step + " needs summary and summary_en"), nil
			}
			run.Repair.Summary = bounded(done.SummaryEN, 1500)
		}
		// The acceptance check is fixed when reproduce or brief approves it:
		// a command sent now is ignored, so the loop cannot move its own goal
		// and a garbled resend cannot burn an attempt.
		if run.Check.IsZero() {
			return refuse("this ceremony has no approved check command"), nil
		}
		result, err := d.Checks.Run(ctx, run.Check)
		if err != nil {
			return StepResult{}, err
		}
		passed := result.Ran && result.ExitCode == 0 && !result.RanNoTests()
		field, success, exhausted := "repaired", "repaired", "repair_exhausted"
		if run.Step == "build" {
			field, success, exhausted = "verified", "verified", "build_exhausted"
		}
		output = map[string]any{"summary": done.Summary, field: passed}
		if run.Repair != nil {
			output["summary_en"] = run.Repair.Summary
		}
		addEvidence(output, report, run.Check, result)
		switch {
		case passed:
			trigger = success
		case run.Iteration >= ceremonyRepeatLimit:
			trigger = exhausted
		default:
			repeat = true
			report["feedback"] = "the check command still fails; fix what its output shows"
			if !result.Ran {
				report["feedback"] = "the check command did not run: " + result.Output
			} else if result.RanNoTests() {
				report["feedback"] = noTestsFeedback
			}
		}
	case "integrate":
		if done.Report == "" || done.SummaryEN == "" {
			return refuse("integrate needs report and summary_en"), nil
		}
		revision := gitAnswer(ctx, d.Checks, "rev-parse", "HEAD")
		dirty := bounded(gitAnswer(ctx, d.Checks, "status", "--porcelain"), 4<<10)
		output = map[string]any{"report": done.Report, "summary_en": done.SummaryEN, "revision": revision, "dirty": dirty, "integrated": true}
		report["revision"] = output["revision"]
		trigger = "integrated"
	case "red", "green":
		if run.Task == nil {
			return refuse("this step belongs to a plan task"), nil
		}
		var refusal string
		if run.Step == "red" {
			output, trigger, repeat, refusal, err = d.taskRed(ctx, &run, done, report)
		} else {
			output, trigger, repeat, refusal, err = d.taskGreen(ctx, s, &run, done, report)
		}
		if err != nil {
			return StepResult{}, err
		}
		if refusal != "" {
			return refuse(refusal), nil
		}
	case "decompose":
		var refusal string
		output, trigger, repeat, refusal, err = d.decompose(ctx, s, run, done, report)
		if err != nil {
			return StepResult{}, err
		}
		if refusal != "" {
			return refuse(refusal), nil
		}
	case "triage", "timeline", "analysis", "publish":
		var refusal string
		output, trigger, refusal, err = d.incidentStep(ctx, &run, done)
		if err != nil {
			return StepResult{}, err
		}
		if refusal != "" {
			return refuse(refusal), nil
		}
	default:
		return refuse("unknown ceremony step " + run.Step), nil
	}
	if run.Compact {
		appendLedger(&run, s, output)
		compactCheckEvidence(report)
	}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("complete %s: %w", run.Step, err))
	}
	if repeat {
		run.Iteration++
		if err := d.claim(ctx, &run); err != nil {
			return d.reconcile(ctx, s, run, output, report, err)
		}
		report["next_step"], report["instruction"] = run.Step, stepInstruction(run)+stepProgress(run)
		d.observe(run, "", report, false, false)
		return accept(report, &run), nil
	}
	state, err := d.Engine.Transition(ctx, run.Instance, trigger)
	if err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("transition %s: %w", trigger, err))
	}
	result, err := d.enter(ctx, s, run, state, output, report)
	if err != nil {
		return d.reconcile(ctx, s, run, output, report, err)
	}
	return result, nil
}

// enter moves the run into state: it closes a terminal ceremony or claims the
// state's step.
func (d *CeremonyDriver) enter(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string, output, report map[string]any) (StepResult, error) {
	report["state"] = state
	if terminalStates[state] {
		if run.Plan != nil {
			report["memory"] = d.finishPlan(ctx, s, run, state)
		} else if run.Task != nil {
			memory, err := d.finishTask(ctx, s, run, state, output)
			if err != nil {
				return StepResult{}, err
			}
			report["memory"] = memory
		} else {
			report["memory"] = d.record(ctx, s, run, state, output)
		}
		report["ceremony"] = state
		report["instruction"] = "The ceremony is over and the session is back in normal mode. Tell the user the outcome in their language."
		if run.Plan != nil && state == "READY" {
			report["instruction"] = "The plan is approved and the console now runs its tasks in separate worker sessions in this workspace. Do not read, run or change anything: answer the user in one or two sentences in their language and end your turn."
		}
		d.observe(run, state, report, true, false)
		return accept(report, nil), nil
	}
	next, ok := stateSteps[state]
	if !ok {
		return StepResult{}, fmt.Errorf("ceremony entered unknown state %s", state)
	}
	run.Step, run.Iteration = next, 1
	return d.enterStep(ctx, s, run, report)
}

// enterStep claims run.Step. Console steps never reach the model: present
// waits for the person and review runs the reviewer.
func (d *CeremonyDriver) enterStep(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	switch run.Step {
	case "present":
		if run.Plan != nil {
			return d.awaitPlan(ctx, s, run, report)
		}
		return d.awaitPerson(ctx, run, report)
	case "handback":
		if run.Task != nil {
			return d.taskHandback(ctx, s, run, report)
		}
	case "review":
		return d.reviewDraft(ctx, s, run, report)
	}
	if repairConsoleSteps[run.Step] {
		return d.repairStep(ctx, s, run, report)
	}
	if err := d.claim(ctx, &run); err != nil {
		return StepResult{}, err
	}
	report["next_step"], report["instruction"] = run.Step, stepInstruction(run)+incidentInstruction(run)+repairInstruction(run)+stepProgress(run)
	d.observe(run, "", report, false, false)
	return accept(report, &run), nil
}

// stepInstruction is the next step's instruction under the run's profile.
func stepInstruction(run domain.CeremonyRun) string {
	if step, ok := compactSteps[run.Step]; ok && run.Compact {
		return step.instruction
	}
	return baseInstruction(run)
}

// baseInstruction is the standard instruction for the run's step; an
// improvement words brief and build its own way.
func baseInstruction(run domain.CeremonyRun) string {
	if text, ok := improveInstructions[run.Step]; ok && improving(run) {
		return text
	}
	return stepInstructions[run.Step]
}

func improving(run domain.CeremonyRun) bool { return run.Repair != nil && run.Repair.Improvement }

// reconcile recovers from an advance that was interrupted after MADE may
// already have recorded part of it: a completion whose answer was lost, a
// completed step whose transition never ran, or a session resumed on a step
// MADE has left. It reads the instance and continues from what MADE says,
// using get_ceremony_instance's own enabled flags rather than re-deriving
// guards. When nothing explains the failure it returns the original error.
func (d *CeremonyDriver) reconcile(ctx context.Context, s domain.Session, run domain.CeremonyRun, output, report map[string]any, cause error) (StepResult, error) {
	view, err := d.Engine.Inspect(ctx, run.Instance)
	if err != nil {
		return StepResult{}, errors.Join(cause, fmt.Errorf("inspect %s: %w", run.Instance, err))
	}
	report["reconciled"] = cause.Error()
	hydrateRepair(&run, view)
	switch {
	case view.State == "COMPLETED" || view.State == "BLOCKED":
		return d.enter(ctx, s, run, view.State, output, report)
	case len(view.Enabled) == 1:
		state, err := d.Engine.Transition(ctx, run.Instance, view.Enabled[0])
		if err != nil {
			return StepResult{}, errors.Join(cause, fmt.Errorf("transition %s: %w", view.Enabled[0], err))
		}
		return d.enter(ctx, s, run, state, output, report)
	case len(view.Enabled) > 1:
		return StepResult{}, errors.Join(cause, fmt.Errorf("MADE has several enabled transitions (%s); resolve the instance by hand", strings.Join(view.Enabled, ", ")))
	}
	step, ok := stateSteps[view.State]
	if ok && view.State == "REVIEW" && !slices.Contains(view.Claimable, step) && slices.Contains(view.Claimable, "review") {
		step = "review" // the draft was handed in; its review never ran
	}
	if ok && repairConsoleSteps[step] && run.Repair != nil {
		if view.LiveErr != nil {
			// Whether our own claim is live is unknown: claiming the step
			// again or passing over it would both be guesses.
			return StepResult{}, errors.Join(cause, view.LiveErr)
		}
		if fence := view.Live[step]; fence != "" {
			// Our own claim is still live: finish the console step with it.
			run.Step, run.Iteration = step, 1
			result, err := d.repairStepWith(ctx, s, run, report, fence)
			if err != nil {
				return StepResult{}, errors.Join(cause, err)
			}
			return result, nil
		}
	}
	if !ok || !slices.Contains(view.Claimable, step) {
		return StepResult{}, cause
	}
	switch {
	case step == "review":
		run.Step = step
	case step == run.Step:
		run.Iteration++ // the completion landed and its repeat condition was false
	default:
		run.Step, run.Iteration = step, 1
	}
	result, err := d.enterStep(ctx, s, run, report)
	if err != nil {
		return StepResult{}, errors.Join(cause, err)
	}
	return result, nil
}

func (d *CeremonyDriver) claim(ctx context.Context, run *domain.CeremonyRun) error {
	return d.claimFor(ctx, run, 0)
}

func (d *CeremonyDriver) claimFor(ctx context.Context, run *domain.CeremonyRun, lease time.Duration) error {
	fence, err := d.Engine.Claim(ctx, run.Instance, run.Step, claimKey(*run), lease)
	if err != nil {
		return fmt.Errorf("claim %s: %w", run.Step, err)
	}
	run.Fence = fence
	run.Reminded = false
	return nil
}

// stepReminder is the console's message when a turn ended with the step open.
func stepReminder(run domain.CeremonyRun) root.Text {
	return root.Text(fmt.Sprintf("[AXLR] The %s step of ceremony %s is still open. If its work is done, hand it back now with axlr_step_done. %s%s", run.Step, run.Definition, stepInstruction(run), stepProgress(run)))
}

// record writes the outcome to the selected project about, or the session
// fallback. Memory is best effort: MADE already holds the durable record.
func (d *CeremonyDriver) record(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string, output map[string]any) string {
	if d.Memory == nil {
		return "not recorded: KMP is not connected"
	}
	about := run.About
	if run.Repair != nil {
		return d.recordRepair(ctx, s, run, state, output)
	}
	if d.Labels != nil {
		labels, err := d.Labels.Load(ctx)
		if err != nil {
			return "not recorded: " + bounded(err.Error(), 300)
		}
		if selected := labels[s.Export().ID].About; selected != "" {
			about = selected
		}
	}
	summary := fmt.Sprintf("%s %s ended %s at step %s.", run.Definition, run.Version, state, run.Step)
	if text, ok := output["summary_en"].(string); ok && text != "" {
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
	if i := run.Incident; i != nil {
		labels["incident"], labels["service"], labels["severity"] = []string{i.Slug}, []string{i.Service}, []string{i.Severity}
		if i.Published != "" {
			evidence += "; postmortem " + i.Published + " sha256 " + i.DraftDigest
		}
	}
	if err := d.Memory.Record(ctx, about, labels, run.Instance, summary, evidence); err != nil {
		return "not recorded: " + bounded(err.Error(), 300)
	}
	return "recorded in " + about
}

func addEvidence(output, report map[string]any, command domain.CheckCommand, result CheckResult) {
	evidence := map[string]any{"program": command.Program, "args": command.Args, "exit_code": result.ExitCode, "output_tail": result.Output}
	if result.RanNoTests() {
		evidence["ran_no_tests"] = true
	}
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

// recordRepair writes the terminal outcome of a repair to the project about:
// a success path with the merged pull request, or an observation of why it
// stopped. The cause was written when diagnose was accepted.
func (d *CeremonyDriver) recordRepair(ctx context.Context, s domain.Session, run domain.CeremonyRun, state string, output map[string]any) string {
	r := run.Repair
	kind := "observation"
	what := "Repair"
	if r.Improvement {
		what = "Improvement"
	}
	summary := fmt.Sprintf("%s %q of %s ended %s at step %s.", what, r.Slug, r.Repository, state, run.Step)
	if state == "COMPLETED" && r.MergeSHA != "" {
		kind = "success_path"
		summary = fmt.Sprintf("%s %q of %s merged as pull request #%d (%s).", what, r.Slug, r.Repository, r.PullRequest, r.MergeSHA)
	} else if reason, _ := output["reason"].(string); reason != "" {
		summary += " Reason: " + bounded(reason, 600)
	} else if errText, _ := output["error"].(string); errText != "" {
		summary += " Error: " + bounded(errText, 600)
	} else if observed, _ := output["observed"].(string); observed != "" && r.Improvement {
		summary += " Not feasible: " + bounded(observed, 600)
	}
	if r.Criteria != "" {
		summary += " Criteria: " + bounded(r.Criteria, 600)
	}
	if r.Summary != "" {
		summary += " " + r.Summary
	}
	evidence := fmt.Sprintf("MADE instance %s", run.Instance)
	if r.URL != "" {
		evidence += "; pull request " + r.URL
	}
	if r.HeadSHA != "" {
		evidence += "; head " + r.HeadSHA
	}
	if !run.Check.IsZero() {
		evidence += "; check " + run.Check.Program + " " + strings.Join(run.Check.Args, " ")
	}
	var links []MemoryLink
	if r.CauseRef != "" {
		links = append(links, MemoryLink{Ref: r.CauseRef, Rel: "follows", Why: "the repair applied the fix proposed for this diagnosed cause", Evidence: evidence})
	}
	record := MemoryRecord{ID: run.Instance + "-outcome", Kind: kind, Summary: summary, Evidence: evidence, Links: links}
	if _, err := d.Memory.RecordLinked(ctx, run.About, repairLabels(s, run), record); err != nil {
		if len(record.Links) == 0 {
			return "not recorded: " + bounded(err.Error(), 300)
		}
		// Keep the fact even when KMP doubts the link.
		record.Links = nil
		if _, retry := d.Memory.RecordLinked(ctx, run.About, repairLabels(s, run), record); retry != nil {
			return "not recorded: " + bounded(retry.Error(), 300)
		}
		return "recorded in " + run.About + " (cause link refused: " + bounded(err.Error(), 200) + ")"
	}
	return "recorded in " + run.About
}
