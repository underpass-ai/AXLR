package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	incidentDir = "docs/incidents"
	// maxIncidentDraft bounds the draft the reviewer reads and MADE records.
	maxIncidentDraft = 64 << 10
	// presentLease covers a person who answers days later.
	presentLease = 7 * 24 * time.Hour
	// maxReviewFailures stops the model retrying a reviewer that keeps
	// failing until the turn budget runs out.
	maxReviewFailures   = 2
	incidentReviewLimit = 2 // REVIEW's max_iterations
	maxFindings         = 12
	maxFindingBytes     = 600
	// maxFindingsBytes keeps the findings well inside the session sidecar.
	maxFindingsBytes = 4 << 10
)

var incidentSlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var incidentSeverities = []string{"sev1", "sev2", "sev3", "sev4"}

func init() {
	for step, text := range map[string]string{
		"triage":   "State what happened, who was affected, for how long and how it was detected, from the evidence in the workspace. Call axlr_step_done with summary, impact, severity (sev1, sev2, sev3 or sev4), detection, service and slug (kebab-case, it names the postmortem file).",
		"timeline": "Reconstruct the timeline from evidence. Call axlr_step_done with timeline: a list of {at (RFC3339 with zone), event, evidence} in time order. Evidence is a workspace path, a log line or a link; the console checks that workspace paths exist.",
		"analysis": "Explain the root cause and the contributing factors without blame: describe systems, signals and decisions, never a person's fault; separate observation from inference. Call axlr_step_done with root_cause, contributing_factors, went_well, went_badly and actions: a list of {title, kind (corrective or preventive), owner, due (YYYY-MM-DD, in the future), verification}.",
		"revise":   "Write the complete blameless postmortem in Markdown with local_write to docs/incidents/<slug>.draft.md: Summary, Impact, Timeline, Root cause, Contributing factors, What went well, What went badly, and an Actions table with owner, due date and verification. Write it in the user's language. Add no status or draft marker: the person approves these exact bytes and the console publishes them unchanged. Then call axlr_step_done with draft_path only. A reviewer in a fresh context judges it; if it rejects the draft you get its findings.",
		"present":  "The draft is with the person for approval. Do not call axlr_step_done; tell the user to read it on the approval card (/incident), where a approves and d sends it back with a reason, and end your turn.",
		"publish":  "The person approved the postmortem and the console wrote it. Write the report for the user in their language: where the postmortem is, the actions with owners and dates, and what remains open. Call axlr_step_done with report and summary_en, two or three plain English sentences for project memory.",
	} {
		stepInstructions[step] = text
	}
}

// incidentReviewRubric is what the fresh-context reviewer judges against.
const incidentReviewRubric = `You review a production incident postmortem. Accept it only if all hold:
1. Blameless: it describes systems, signals and decisions; no person is blamed or named as the cause.
2. Impact is concrete: who was affected, how much and for how long.
3. The timeline has times and evidence, and the analysis follows from it.
4. Observation and inference are kept apart; the root cause is supported, not guessed.
5. Contributing factors and what went well or badly are present.
6. Every action has an owner, a due date and how it will be verified, and the actions address the root cause and the contributing factors.
7. When the person returned the draft with a reason, that reason is addressed.
Findings must be specific and actionable. Do not rewrite the document.`

type incidentDone struct {
	Impact              string           `json:"impact"`
	Severity            string           `json:"severity"`
	Detection           string           `json:"detection"`
	Service             string           `json:"service"`
	Slug                string           `json:"slug"`
	Timeline            []timelineEntry  `json:"timeline"`
	ContributingFactors []string         `json:"contributing_factors"`
	WentWell            []string         `json:"went_well"`
	WentBadly           []string         `json:"went_badly"`
	Actions             []incidentAction `json:"actions"`
	DraftPath           string           `json:"draft_path"`
}

type timelineEntry struct {
	At       string `json:"at"`
	Event    string `json:"event"`
	Evidence string `json:"evidence"`
}

type incidentAction struct {
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	Owner        string `json:"owner"`
	Due          string `json:"due"`
	Verification string `json:"verification"`
}

// incidentInstruction adds what the next revise must address.
func incidentInstruction(run domain.CeremonyRun) string {
	i := run.Incident
	if i == nil || run.Step != "revise" {
		return ""
	}
	text := fmt.Sprintf(" Review round %d of %d.", run.Iteration, incidentReviewLimit)
	if i.Slug != "" {
		text += " Draft path: " + incidentDir + "/" + i.Slug + ".draft.md."
	}
	if i.DraftDigest != "" {
		// Seen on 2 Oct 2026: rewriting the whole draft twice in one turn,
		// once failing for the missing precondition, overran the context.
		text += " The draft already exists: change it with local_edit where you can; to rewrite it whole use local_write with mode replace and expected_sha256 " + i.DraftDigest + "."
	}
	if len(i.Findings) > 0 {
		text += " The reviewer rejected the last draft; address every finding: " + strings.Join(i.Findings, " | ")
	}
	if i.ReturnReason != "" {
		text += " The person sent the draft back: " + i.ReturnReason
	}
	return text
}

// incidentStep checks the model-written incident steps. A non-empty refusal
// leaves MADE untouched.
func (d *CeremonyDriver) incidentStep(ctx context.Context, run *domain.CeremonyRun, done stepDone) (map[string]any, string, string, error) {
	i := run.Incident
	if i == nil {
		return nil, "", "this ceremony has no incident state", nil
	}
	switch run.Step {
	case "triage":
		if done.Summary == "" || done.Impact == "" || done.Detection == "" || done.Service == "" {
			return nil, "", "triage needs summary, impact, severity, detection, service and slug", nil
		}
		if !slices.Contains(incidentSeverities, done.Severity) {
			return nil, "", "severity must be sev1, sev2, sev3 or sev4", nil
		}
		if len(done.Slug) > 64 || !incidentSlug.MatchString(done.Slug) {
			return nil, "", "slug must be kebab-case, lower case, at most 64 characters", nil
		}
		i.Slug, i.Service, i.Severity = done.Slug, done.Service, done.Severity
		return map[string]any{"summary": done.Summary, "impact": done.Impact, "severity": done.Severity, "detection": done.Detection, "service": done.Service, "slug": done.Slug, "triaged": true}, "triaged", "", nil
	case "timeline":
		refusal, err := d.checkTimeline(ctx, done.Timeline)
		if refusal != "" || err != nil {
			return nil, "", refusal, err
		}
		return map[string]any{"timeline": done.Timeline, "timeline_ok": true}, "timelined", "", nil
	case "analysis":
		if done.RootCause == "" || len(done.ContributingFactors) == 0 || done.WentWell == nil || done.WentBadly == nil || len(done.Actions) == 0 {
			return nil, "", "analysis needs root_cause, contributing_factors, went_well, went_badly and at least one action", nil
		}
		today := d.now().UTC().Format(time.DateOnly)
		for n, action := range done.Actions {
			if action.Title == "" || action.Owner == "" || action.Verification == "" {
				return nil, "", fmt.Sprintf("action %d needs title, owner and verification", n+1), nil
			}
			if action.Kind != "corrective" && action.Kind != "preventive" {
				return nil, "", fmt.Sprintf("action %d: kind must be corrective or preventive", n+1), nil
			}
			if _, err := time.Parse(time.DateOnly, action.Due); err != nil || action.Due <= today {
				return nil, "", fmt.Sprintf("action %d: due must be a future date as YYYY-MM-DD (today is %s)", n+1, today), nil
			}
		}
		return map[string]any{"root_cause": done.RootCause, "contributing_factors": done.ContributingFactors, "went_well": done.WentWell, "went_badly": done.WentBadly, "actions": done.Actions, "analyzed": true}, "analyzed", "", nil
	case "publish":
		if done.Report == "" || done.SummaryEN == "" {
			return nil, "", "publish needs report and summary_en", nil
		}
		content, found, err := d.Files.Read(ctx, i.Published, maxIncidentDraft+1)
		if err != nil {
			return nil, "", "", err
		}
		if !found || digestOf(content) != i.DraftDigest {
			return nil, "", "the postmortem file " + i.Published + " no longer holds the approved bytes; restore it before publishing", nil
		}
		return map[string]any{"report": done.Report, "summary_en": done.SummaryEN, "path": i.Published, "draft_digest": i.DraftDigest, "published": true}, "published", "", nil
	}
	return nil, "", "unknown incident step " + run.Step, nil
}

func (d *CeremonyDriver) checkTimeline(ctx context.Context, entries []timelineEntry) (string, error) {
	if len(entries) == 0 {
		return "timeline needs at least one entry", nil
	}
	var previous time.Time
	for n, entry := range entries {
		at, err := time.Parse(time.RFC3339, entry.At)
		if err != nil || entry.Event == "" || entry.Evidence == "" {
			return fmt.Sprintf("timeline entry %d needs at (RFC3339 with zone), event and evidence", n+1), nil
		}
		if at.Before(previous) {
			return fmt.Sprintf("timeline entry %d is earlier than the one before; order the timeline", n+1), nil
		}
		previous = at
		if file, ok := workspaceEvidence(entry.Evidence); ok {
			_, found, err := d.Files.Read(ctx, file, 1)
			if err != nil {
				return "", err
			}
			if !found {
				return fmt.Sprintf("timeline entry %d cites %s, which does not exist in the workspace", n+1, file), nil
			}
		}
	}
	return "", nil
}

// workspaceEvidence reports evidence that names a workspace file, such as
// logs/api.log or logs/api.log#L42; prose, URLs and absolute paths are not
// checked.
func workspaceEvidence(evidence string) (string, bool) {
	file, _, _ := strings.Cut(strings.TrimSpace(evidence), "#")
	if file == "" || strings.ContainsAny(file, " \t:") || strings.HasPrefix(file, "/") {
		return "", false
	}
	if !strings.Contains(file, "/") && path.Ext(file) == "" {
		return "", false
	}
	return path.Clean(file), true
}

func draftPathAllowed(raw string) bool {
	clean := path.Clean(raw)
	return clean == raw && strings.HasPrefix(clean, incidentDir+"/") && strings.HasSuffix(clean, ".draft.md") && !strings.Contains(strings.TrimPrefix(clean, incidentDir+"/"), "/")
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// revise takes the draft, asks the reviewer before touching MADE and only
// then records revise and review together, so a cancelled or failed review
// leaves the model's step open and review unclaimed.
func (d *CeremonyDriver) revise(ctx context.Context, s domain.Session, run domain.CeremonyRun, done stepDone) (StepResult, error) {
	if run.Incident == nil {
		return refuse("this ceremony has no incident state"), nil
	}
	if !draftPathAllowed(done.DraftPath) {
		return refuse("draft_path must be " + incidentDir + "/<slug>.draft.md"), nil
	}
	draft, found, err := d.Files.Read(ctx, done.DraftPath, maxIncidentDraft+1)
	if err != nil {
		return StepResult{}, err
	}
	if !found || len(draft) == 0 {
		return refuse(done.DraftPath + " does not exist; write the draft with local_write first"), nil
	}
	if len(draft) > maxIncidentDraft {
		return refuse(fmt.Sprintf("the draft exceeds %d KiB; tighten it", maxIncidentDraft>>10)), nil
	}
	verdict, result, err := d.askReviewer(ctx, s, &run, draft)
	if err != nil {
		return StepResult{}, err
	}
	if result != nil {
		return *result, nil
	}
	i := run.Incident
	i.DraftPath, i.DraftDigest = done.DraftPath, digestOf(draft)
	output := map[string]any{"draft_path": i.DraftPath, "draft_digest": i.DraftDigest, "draft": string(draft)}
	report := map[string]any{"step": run.Step, "iteration": run.Iteration, "draft_digest": i.DraftDigest}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("complete revise: %w", err))
	}
	run.Step = "review"
	return d.recordReview(ctx, s, run, verdict, report)
}

// reviewDraft runs a review whose revise already landed in MADE, as when a
// resume finds review claimable.
func (d *CeremonyDriver) reviewDraft(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	i := run.Incident
	if i == nil || i.DraftPath == "" {
		return StepResult{}, errors.New("the review has no draft to read")
	}
	draft, found, err := d.Files.Read(ctx, i.DraftPath, maxIncidentDraft+1)
	if err != nil {
		return StepResult{}, err
	}
	if !found || digestOf(draft) != i.DraftDigest {
		return StepResult{}, fmt.Errorf("%s changed after it was handed in; MADE holds the reviewed draft", i.DraftPath)
	}
	verdict, result, err := d.askReviewer(ctx, s, &run, draft)
	if err != nil {
		return StepResult{}, err
	}
	if result != nil {
		return StepResult{}, errors.New("the reviewer failed; resume the ceremony to try again")
	}
	return d.recordReview(ctx, s, run, verdict, report)
}

// askReviewer returns a verdict, or the refusal to give the model, or an
// error when the context was cancelled.
func (d *CeremonyDriver) askReviewer(ctx context.Context, s domain.Session, run *domain.CeremonyRun, draft []byte) (ReviewVerdict, *StepResult, error) {
	i := run.Incident
	verdict, err := d.Reviewer.Review(ctx, ReviewRequest{SessionModel: string(s.Export().Model), Rubric: incidentReviewRubric, Draft: string(draft), ReturnReason: i.ReturnReason})
	if ctx.Err() != nil {
		return ReviewVerdict{}, nil, ctx.Err()
	}
	if err != nil {
		i.ReviewFailures++
		refusal := refuse("the reviewer failed (" + bounded(err.Error(), 300) + "); nothing was recorded, call axlr_step_done again with the same draft_path")
		if i.ReviewFailures >= maxReviewFailures {
			refusal = refuse("the reviewer failed " + fmt.Sprint(i.ReviewFailures) + " times in a row (" + bounded(err.Error(), 300) + "); stop, tell the user the reviewer is unavailable and end your turn")
		}
		kept := *run
		refusal.Run = &kept
		return ReviewVerdict{}, &refusal, nil
	}
	i.ReviewFailures = 0
	return verdict, nil, nil
}

// recordReview claims review with the verdict in hand, completes it and
// advances: to APPROVAL, to another revise round or to BLOCKED.
func (d *CeremonyDriver) recordReview(ctx context.Context, s domain.Session, run domain.CeremonyRun, verdict ReviewVerdict, report map[string]any) (StepResult, error) {
	i := run.Incident
	findings := boundedFindings(verdict.Findings)
	output := map[string]any{"accepted": verdict.Accepted, "findings": findings, "review_mode": "independent_context", "reviewer_model": verdict.Model, "draft_digest": i.DraftDigest}
	report["review"] = output
	run.Step = "review"
	if err := d.claim(ctx, &run); err != nil {
		return StepResult{}, err
	}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("complete review: %w", err))
	}
	i.Findings = nil
	if verdict.Accepted {
		// The person's reason stays until a review accepts the draft.
		i.ReturnReason = ""
		state, err := d.Engine.Transition(ctx, run.Instance, "reviewed")
		if err != nil {
			return d.reconcile(ctx, s, run, output, report, fmt.Errorf("transition reviewed: %w", err))
		}
		return d.enter(ctx, s, run, state, output, report)
	}
	i.Findings = findings
	view, err := d.Engine.Inspect(ctx, run.Instance)
	if err != nil {
		return StepResult{}, fmt.Errorf("inspect %s: %w", run.Instance, err)
	}
	switch {
	case slices.Contains(view.Enabled, "review_exhausted"):
		state, err := d.Engine.Transition(ctx, run.Instance, "review_exhausted")
		if err != nil {
			return StepResult{}, fmt.Errorf("transition review_exhausted: %w", err)
		}
		return d.enter(ctx, s, run, state, output, report)
	case slices.Contains(view.Claimable, "revise"):
		run.Step = "revise"
		run.Iteration++
		return d.enterStep(ctx, s, run, report)
	}
	return StepResult{}, fmt.Errorf("after a rejected review MADE offers neither another round nor review_exhausted (state %s)", view.State)
}

func boundedFindings(findings []string) []string {
	out := make([]string, 0, min(len(findings), maxFindings))
	total := 0
	for _, finding := range findings {
		finding = bounded(strings.TrimSpace(finding), maxFindingBytes)
		if finding == "" || len(out) == maxFindings || total+len(finding) > maxFindingsBytes {
			continue
		}
		total += len(finding)
		out = append(out, finding)
	}
	return out
}

// awaitPerson claims present for the person and stops asking the model.
func (d *CeremonyDriver) awaitPerson(ctx context.Context, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	if err := d.claimFor(ctx, &run, presentLease); err != nil {
		return StepResult{}, err
	}
	i := run.Incident
	i.Awaiting, i.Decided, i.Granted = domain.AwaitingApproval, "", false
	report["next_step"], report["instruction"] = run.Step, stepInstructions[run.Step]
	return accept(report, &run), nil
}

// Draft returns the draft awaiting the person and whether it still holds the
// reviewed bytes.
func (d *CeremonyDriver) Draft(ctx context.Context, s domain.Session) (string, bool, error) {
	run, live := s.Ceremony()
	if !live || !run.AwaitingPerson() || d == nil || d.Files == nil {
		return "", false, errors.New("no draft is waiting for approval")
	}
	draft, found, err := d.Files.Read(ctx, run.Incident.DraftPath, maxIncidentDraft+1)
	if err != nil {
		return "", false, err
	}
	return string(draft), found && digestOf(draft) == run.Incident.DraftDigest, nil
}

// Approve is the person's a: complete present, then grant the guard as the
// approver, write the approved bytes and apply approved. Completing first
// means a failed grant leaves nothing approved in MADE. Each part that landed
// is kept in the returned run, so pressing a again resumes after it.
func (d *CeremonyDriver) Approve(ctx context.Context, s domain.Session) (StepResult, error) {
	run, i, err := d.awaiting(s)
	if err != nil {
		return StepResult{}, err
	}
	if run.Repair != nil {
		return d.approveMerge(ctx, s, run)
	}
	if i.Decided == "return" {
		return StepResult{}, errors.New("MADE already recorded a return; press d to send the draft back")
	}
	draft, found, err := d.Files.Read(ctx, i.DraftPath, maxIncidentDraft+1)
	if err != nil {
		return StepResult{}, err
	}
	if !found || digestOf(draft) != i.DraftDigest {
		return StepResult{}, errors.New("the draft changed after review; press d to send it back")
	}
	keep := func(err error) (StepResult, error) { return StepResult{Run: &run}, err }
	if i.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "approve", "draft_digest": i.DraftDigest})
		if err != nil {
			return StepResult{}, err
		}
		i.Decided = decided
		if decided != "approve" {
			return keep(errors.New("MADE already recorded a return; press d to send the draft back"))
		}
	}
	if !i.Granted {
		if err := d.Approver.ApproveGuard(ctx, run.Instance, "person_approves"); err != nil {
			return keep(fmt.Errorf("approve as the person: %w", err))
		}
		i.Granted = true
	}
	if i.Published == "" {
		i.Published = fmt.Sprintf("%s/%s-%s.md", incidentDir, d.now().UTC().Format(time.DateOnly), i.Slug)
	}
	if err := d.Files.Write(ctx, i.Published, draft); err != nil {
		return keep(fmt.Errorf("write %s: %w", i.Published, err))
	}
	report := map[string]any{"step": run.Step, "decision": "approve", "postmortem": i.Published}
	before := copyRun(run)
	i.Awaiting = ""
	return d.leaveApproval(ctx, s, run, before, "approved", report)
}

// leaveApproval applies the person's transition. If neither it nor the
// reconciliation lands, the run from before is kept, still awaiting the
// person, with every part MADE already recorded.
func (d *CeremonyDriver) leaveApproval(ctx context.Context, s domain.Session, run, before domain.CeremonyRun, trigger string, report map[string]any) (StepResult, error) {
	state, err := d.Engine.Transition(ctx, run.Instance, trigger)
	if err == nil {
		result, err := d.enter(ctx, s, run, state, nil, report)
		if err != nil {
			return StepResult{Run: &before}, err
		}
		return result, nil
	}
	result, err := d.reconcile(ctx, s, run, nil, report, fmt.Errorf("transition %s: %w", trigger, err))
	if err != nil {
		return StepResult{Run: &before}, err
	}
	return result, nil
}

func copyRun(run domain.CeremonyRun) domain.CeremonyRun {
	if run.Incident != nil {
		incident := *run.Incident
		incident.Findings = slices.Clone(incident.Findings)
		run.Incident = &incident
	}
	if run.Repair != nil {
		repair := *run.Repair
		repair.WakeRefs = slices.Clone(repair.WakeRefs)
		run.Repair = &repair
	}
	return run
}

// Return is the person's d: complete present with the reason, apply returned
// and give the draft back to revise. The console allows two returns.
func (d *CeremonyDriver) Return(ctx context.Context, s domain.Session, reason string) (StepResult, error) {
	run, i, err := d.awaiting(s)
	if err != nil {
		return StepResult{}, err
	}
	if run.Repair != nil {
		return d.declineMerge(ctx, s, run, reason)
	}
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return StepResult{}, errors.New("say why the draft goes back")
	case i.Decided == "approve":
		return StepResult{}, errors.New("MADE already recorded the approval; press a to finish it")
	case i.Returns >= domain.MaxIncidentReturns && i.Decided == "":
		return StepResult{}, fmt.Errorf("the draft has gone back %d times; approve it or stop the ceremony", i.Returns)
	}
	reason = bounded(reason, 2000)
	if i.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "APPROVAL", map[string]any{"decision": "return", "reason": reason, "draft_digest": i.DraftDigest})
		if err != nil {
			return StepResult{}, err
		}
		if decided != "return" {
			i.Decided = decided
			return StepResult{Run: &run}, errors.New("MADE already recorded the approval; press a to finish it")
		}
		// Counted now: the next REVIEW visit's claim keys depend on it.
		i.Decided, i.Returns, i.ReturnReason = "return", i.Returns+1, reason
	}
	report := map[string]any{"step": run.Step, "decision": "return", "reason": i.ReturnReason}
	before := copyRun(run)
	i.Awaiting, i.Decided, i.Findings = "", "", nil
	return d.leaveApproval(ctx, s, run, before, "returned", report)
}

// completeDecision records the person's decision on the current step. When
// the completion fails it asks MADE whether an earlier one already landed in
// this state, and returns that decision, so a lost answer does not leave both
// keys failing forever.
func (d *CeremonyDriver) completeDecision(ctx context.Context, run domain.CeremonyRun, state string, output map[string]any) (string, error) {
	err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output)
	if err == nil {
		return output["decision"].(string), nil
	}
	view, inspectErr := d.Engine.Inspect(ctx, run.Instance)
	if inspectErr != nil {
		return "", errors.Join(fmt.Errorf("complete %s: %w", run.Step, err), inspectErr)
	}
	if landed, ok := view.Completed[run.Step]; ok && view.State == state {
		if decision, _ := landed["decision"].(string); decision != "" {
			return decision, nil
		}
	}
	return "", fmt.Errorf("complete %s: %w", run.Step, err)
}

func (d *CeremonyDriver) awaiting(s domain.Session) (domain.CeremonyRun, *domain.IncidentRun, error) {
	run, live := s.Ceremony()
	if !live || !run.AwaitingPerson() {
		return domain.CeremonyRun{}, nil, errors.New("nothing is waiting for approval")
	}
	if d == nil || d.Engine == nil || d.Approver == nil || (run.Incident != nil && d.Files == nil) {
		return domain.CeremonyRun{}, nil, errors.New("MADE is not connected; the decision cannot be recorded")
	}
	return run, run.Incident, nil
}

// CanReturn reports whether the person may still send the draft back, or
// decline the merge.
func CanReturn(run domain.CeremonyRun) bool {
	if run.Repair != nil {
		return CanDecline(run)
	}
	return run.Incident != nil && run.Incident.Decided != "approve" && (run.Incident.Returns < domain.MaxIncidentReturns || run.Incident.Decided == "return")
}
