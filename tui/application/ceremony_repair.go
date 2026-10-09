package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	// RepairMarker is the file the launcher writes in a repair clone; its
	// presence is what lets /repair start there, or /improve when its kind
	// is ImproveKind.
	RepairMarker = ".axlr-repair.json"
	// ImproveKind marks a clone prepared by --improve.
	ImproveKind = "improve"
	// repairPoll and repairWatch are the defaults when settings.json is silent.
	repairPoll        = 30 * time.Second
	repairWatch       = 45 * time.Minute
	maxRepairFeedback = 4 << 10
)

// RepairMarkerFile is the launcher's record of what the clone is for.
type RepairMarkerFile struct {
	Version    int    `json:"version"`
	Repository string `json:"repository"`
	Base       string `json:"base"`
	Slug       string `json:"slug"`
	Brief      string `json:"brief"`
	About      string `json:"about"`
	Issue      string `json:"issue,omitempty"`
	// Kind is ImproveKind for an improvement clone, empty for a repair.
	Kind    string `json:"kind,omitempty"`
	Created string `json:"created"`
	// Origin is the session that requested the repair from a running
	// console, empty for a launcher clone; Build the console build that
	// detected the defect.
	Origin string `json:"origin_session,omitempty"`
	Build  string `json:"build,omitempty"`
}

// repairConsoleSteps never reach the model.
var repairConsoleSteps = map[string]bool{"propose": true, "watch": true, "decide": true, "merge": true}

func init() {
	for state, step := range map[string]string{"PROPOSE": "propose", "WATCH": "watch", "DECIDE": "decide", "MERGE": "merge"} {
		stateSteps[state] = step
	}
	stepInstructions["propose"] = "The console commits the clone's changes, pushes the repair branch and opens the pull request; no model work."
	stepInstructions["watch"] = "The console watches the pull request's checks; no model work."
	stepInstructions["decide"] = "The console records whether the merge is automatic or waits for the person; no model work."
	stepInstructions["merge"] = "The console merges the pull request; no model work."
}

// repairInstruction adds what the repair ceremony needs the model to know.
func repairInstruction(run domain.CeremonyRun) string {
	r := run.Repair
	if r == nil {
		return ""
	}
	if r.Improvement {
		return improveInstruction(run)
	}
	var text strings.Builder
	fmt.Fprintf(&text, " Repair of %s in this clone (branch %s).", r.Repository, r.Branch)
	switch run.Step {
	case "diagnose":
		text.WriteString(" You may add connect_to: a list of {ref, rel, why} naming memory refs shown in the recall below, so the cause is linked to what the project already knows; refs not shown there are refused.")
	case "repair":
		text.WriteString(" Also return summary_en: two or three plain English sentences for project memory. Do not commit, push or open pull requests yourself: the console does that once the check passes.")
		if r.Rounds > 0 && r.Feedback != "" {
			fmt.Fprintf(&text, " Check round %d of %d came back red on pull request #%d; failing checks: %s", r.Rounds, domain.MaxRepairRounds, r.PullRequest, r.Feedback)
		}
	}
	if len(r.WakeRefs) > 0 && (run.Step == "diagnose" || run.Step == "repair") {
		text.WriteString(" Memory refs you may link to: " + strings.Join(r.WakeRefs, ", ") + ".")
	}
	return text.String()
}

// memoryRelations are the relation types KMP accepts; a link with another
// name is refused here so one invented word does not sink the whole write.
var memoryRelations = map[string]bool{}

func init() {
	for _, rel := range strings.Fields("follows answers uses_background depends_on chosen_because triggers authorizes verified_by semantic_delta_from updates_state supports supersedes contradicts satisfies_constraint violates_constraint contributes_to excluded_from checked_against derived_from confirms_selection restates corrects component_of total_of same_event_as same_entity_as qualifies_as matches_requirement contains member_of scoped_to") {
		memoryRelations[rel] = true
	}
}

type memoryLinkArgument struct {
	Ref      string `json:"ref"`
	Rel      string `json:"rel"`
	Why      string `json:"why"`
	Evidence string `json:"evidence"`
}

// repairLinks keeps only the links whose ref the recall exposed. The refused
// ones are named so the model learns the rule instead of guessing refs.
func repairLinks(run domain.CeremonyRun, proposed []memoryLinkArgument) ([]MemoryLink, []string) {
	var links []MemoryLink
	var refused []string
	for _, link := range proposed {
		ref, rel, why := strings.TrimSpace(link.Ref), strings.TrimSpace(link.Rel), strings.TrimSpace(link.Why)
		if ref == "" || !memoryRelations[rel] || why == "" || run.Repair == nil || !slices.Contains(run.Repair.WakeRefs, ref) {
			refused = append(refused, ref+" ("+rel+")")
			continue
		}
		links = append(links, MemoryLink{Ref: ref, Rel: rel, Why: bounded(why, 600), Evidence: bounded(strings.TrimSpace(link.Evidence), 600)})
	}
	return links, refused
}

// beginRepair reads the clone's marker and the focused recall. It refuses a
// workspace the launcher did not prepare for this mode, because the console
// would otherwise push a branch from an arbitrary directory.
func (d *CeremonyDriver) beginRepair(ctx context.Context, s *domain.Session, run *domain.CeremonyRun) (map[string]string, error) {
	improve := s.Mode() == domain.ModeImprove
	kind, clone, launch := "repair", "a repair clone", `axlr-tui --repair "<failure brief or #issue>"`
	if improve {
		kind, clone, launch = "improvement", "an improvement clone", `axlr-tui --improve "<improvement brief or #issue>"`
	}
	if d.Files == nil || d.Forge == nil {
		return nil, fmt.Errorf("the %s ceremony needs workspace files and a forge; prepare MADE: open /mcp, select MADE and press p", kind)
	}
	data, found, err := d.Files.Read(ctx, RepairMarker, 16<<10)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", RepairMarker, err)
	}
	if !found {
		return nil, fmt.Errorf("this workspace is not %s; start one with: %s", clone, launch)
	}
	var marker RepairMarkerFile
	if err := json.Unmarshal(data, &marker); err != nil || marker.Version != 1 || marker.Repository == "" || marker.Slug == "" {
		return nil, fmt.Errorf("%s is not a valid repair marker", RepairMarker)
	}
	if (marker.Kind == ImproveKind) != improve {
		return nil, fmt.Errorf("this workspace is not %s: its marker was written for the other ceremony; start one with: %s", clone, launch)
	}
	if marker.Base == "" {
		marker.Base = "main"
	}
	about := marker.About
	if about == "" {
		about = "project:" + strings.ToLower(marker.Repository[strings.LastIndex(marker.Repository, "/")+1:])
	}
	title, _, _ := strings.Cut(strings.TrimSpace(marker.Brief), "\n")
	branch, prefix := "repair/", "Repair: "
	if improve {
		branch, prefix = "improve/", "Improve: "
	}
	repair := &domain.RepairRun{Improvement: improve, Repository: marker.Repository, Base: marker.Base, Slug: marker.Slug, Branch: branch + marker.Slug, Title: titleWithin(prefix, strings.TrimSpace(title))}
	if repair.Title == prefix {
		repair.Title = titleWithin(prefix, strings.ReplaceAll(marker.Slug, "-", " "))
	}
	run.About = about
	run.Repair = repair
	if d.Memory != nil {
		text, refs, err := d.Memory.WakeFocused(ctx, about, marker.Brief)
		switch {
		case err != nil:
			run.Memory = "KMP recall unavailable: " + bounded(err.Error(), 300)
		case len(text) > maxCeremonyMemoryBytes:
			prefix := "Partial KMP recall: console context shortened; recover the full about with kmp_wake before relying on omitted evidence.\n"
			run.Memory = prefix + bounded(text, maxCeremonyMemoryBytes-len(prefix))
		default:
			run.Memory = text
		}
		repair.WakeRefs = refs
		if session, err := d.Memory.Wake(ctx, "ws:"+string(s.Export().ID)); err == nil && session != "" {
			run.Memory += "\nSession recall: " + bounded(session, 1<<10)
		}
	}
	inputs := map[string]string{"repository": marker.Repository}
	if marker.Issue != "" {
		inputs["issue"] = marker.Issue
	}
	return inputs, nil
}

// recordCause writes the diagnosis to the project about with the links the
// model proposed. Best effort: the step is accepted even if memory refuses.
func (d *CeremonyDriver) recordCause(ctx context.Context, s domain.Session, run *domain.CeremonyRun, done stepDone, report map[string]any) {
	r := run.Repair
	r.Cause, r.Fix = bounded(done.RootCause, 1500), bounded(done.ProposedFix, 1500)
	if d.Memory == nil {
		report["memory"] = "not recorded: KMP is not connected"
		return
	}
	links, refused := repairLinks(*run, done.ConnectTo)
	if len(refused) > 0 {
		report["links_refused"] = refused
	}
	record := MemoryRecord{ID: run.Instance + "-cause", Kind: "error_path",
		Summary:  fmt.Sprintf("Cause of %q in %s: %s Proposed fix: %s", bounded(r.Slug, 80), r.Repository, r.Cause, r.Fix),
		Evidence: bounded(done.Evidence, 1500) + fmt.Sprintf(" (MADE instance %s)", run.Instance), Links: links}
	labels := repairLabels(s, *run)
	labels["step"] = []string{"diagnose"}
	ref, err := d.Memory.RecordLinked(ctx, run.About, labels, record)
	if err != nil && len(record.Links) > 0 {
		// Keep the cause even when KMP doubts a link; say which part failed.
		report["links_refused"] = append(toStrings(report["links_refused"]), "all: "+bounded(err.Error(), 300))
		record.Links = nil
		ref, err = d.Memory.RecordLinked(ctx, run.About, labels, record)
	}
	if err != nil {
		report["memory"] = "cause not recorded: " + bounded(err.Error(), 300)
		return
	}
	r.CauseRecorded, r.CauseRef = true, ref
	report["memory"] = fmt.Sprintf("cause recorded in %s with %d links", run.About, len(links))
}

func toStrings(value any) []string {
	out, _ := value.([]string)
	return out
}

func repairLabels(s domain.Session, run domain.CeremonyRun) map[string][]string {
	labels := map[string][]string{"ceremony": {run.Definition}, "step": {run.Step}, "ws": {string(s.Export().Workspace)}, "session": {string(s.Export().ID)}}
	if r := run.Repair; r != nil {
		labels[r.Kind()], labels["repository"] = []string{r.Slug}, []string{r.Repository}
		if r.PullRequest > 0 {
			labels["pull_request"] = []string{fmt.Sprintf("%d", r.PullRequest)}
		}
	}
	return labels
}

// repairStep runs one console step of the repair ceremony: it claims the
// step, does the forge work, completes it and applies the transition. It
// chains into the next console step until the model is needed again, the
// person is needed, or the ceremony ends.
func (d *CeremonyDriver) repairStep(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	return d.repairStepWith(ctx, s, run, report, "")
}

// repairStepWith runs the console step; a nonempty fence is a live claim of
// ours recovered from MADE, so the step is not claimed again.
func (d *CeremonyDriver) repairStepWith(ctx context.Context, s domain.Session, run domain.CeremonyRun, report map[string]any, fence string) (StepResult, error) {
	if run.Repair == nil || d.Forge == nil {
		return StepResult{}, errors.New("the repair ceremony has no forge")
	}
	r := run.Repair
	lease := time.Duration(0)
	switch {
	case run.Step == "watch":
		lease = d.watchDeadline() + 15*time.Minute
	case run.Step == "decide" && !d.automaticMerge(run):
		lease = presentLease // the person may answer days later
	}
	if fence != "" {
		run.Fence, run.Reminded = fence, false
		report["reused_claim"] = run.Step
	} else if err := d.claimFor(ctx, &run, lease); err != nil {
		return StepResult{}, err
	}
	d.observe(run, "", report, false, false)
	var output map[string]any
	var trigger string
	switch run.Step {
	case "propose":
		trailer := "Repaired-by"
		if r.Improvement {
			trailer = "Improved-by"
		}
		pr, err := d.Forge.Propose(ctx, RepairProposal{Repository: r.Repository, Base: r.Base, Branch: r.Branch, Number: r.PullRequest,
			Title: proposalTitle(*r), Body: repairBody(run), Trailer: fmt.Sprintf("%s: AXLR %s %s %s", trailer, run.Definition, run.Version, run.Instance)})
		if err != nil {
			if ctx.Err() != nil {
				return StepResult{}, err
			}
			output, trigger = map[string]any{"proposed": false, "error": bounded(err.Error(), 2000)}, "propose_failed"
			break
		}
		r.PullRequest, r.URL, r.HeadSHA = pr.Number, pr.URL, pr.HeadSHA
		output, trigger = map[string]any{"proposed": true, "pull_request": pr.Number, "url": pr.URL, "head_sha": pr.HeadSHA, "round": r.Rounds + 1}, "proposed"
		report["pull_request"], report["url"] = pr.Number, pr.URL
	case "watch":
		verdict, detail, err := d.watch(ctx, run)
		if err != nil {
			return StepResult{}, err
		}
		output = map[string]any{"verdict": verdict, "pull_request": r.PullRequest, "head_sha": r.HeadSHA, "round": r.Rounds + 1}
		for key, value := range detail {
			output[key] = value
		}
		report["watch"] = output
		switch verdict {
		case "green":
			trigger = "checks_passed"
		case "red":
			r.Rounds++
			if failed, _ := detail["failed"].([]string); len(failed) > 0 {
				r.Feedback = bounded(strings.Join(failed, "; "), maxRepairFeedback)
			}
			trigger = "checks_failed"
		default:
			trigger = "watch_blocked"
		}
	case "decide":
		if !d.automaticMerge(run) {
			return d.awaitMerge(run, report)
		}
		output, trigger = map[string]any{"decision": "automatic", "pull_request": r.PullRequest}, "merge_automatic"
		r.Decided = "automatic"
	case "merge":
		sha, err := d.Forge.Merge(ctx, r.Repository, r.PullRequest)
		if err != nil {
			if ctx.Err() != nil {
				return StepResult{}, err
			}
			output, trigger = map[string]any{"merged": false, "pull_request": r.PullRequest, "error": bounded(err.Error(), 2000)}, "merge_failed"
			break
		}
		r.MergeSHA = sha
		output, trigger = map[string]any{"merged": true, "pull_request": r.PullRequest, "merge_sha": sha}, "merged"
		report["merge_sha"] = sha
	default:
		return StepResult{}, fmt.Errorf("%s is not a console step", run.Step)
	}
	if err := d.Engine.Complete(ctx, run.Instance, run.Step, run.Fence, output); err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("complete %s: %w", run.Step, err))
	}
	state, err := d.Engine.Transition(ctx, run.Instance, trigger)
	if err != nil {
		return d.reconcile(ctx, s, run, output, report, fmt.Errorf("transition %s: %w", trigger, err))
	}
	return d.enter(ctx, s, run, state, output, report)
}

// watch polls the pull request until its checks settle. BEHIND is repaired
// once per round by updating the branch; anything the console cannot read as
// progress within the deadline is blocked, never guessed green.
func (d *CeremonyDriver) watch(ctx context.Context, run domain.CeremonyRun) (string, map[string]any, error) {
	r := run.Repair
	deadline := d.now().Add(d.watchDeadline())
	updated := false
	polls := 0
	for {
		status, err := d.Forge.Status(ctx, r.Repository, r.PullRequest)
		if err != nil {
			if ctx.Err() != nil {
				return "", nil, err
			}
			return "blocked", map[string]any{"reason": "status unreadable: " + bounded(err.Error(), 1000)}, nil
		}
		polls++
		if status.HeadSHA != "" {
			r.HeadSHA = status.HeadSHA
		}
		switch {
		case status.State == "MERGED":
			return "blocked", map[string]any{"reason": "the pull request was merged outside the ceremony"}, nil
		case status.State == "CLOSED":
			return "blocked", map[string]any{"reason": "the pull request was closed outside the ceremony"}, nil
		case len(status.Failed) > 0 && status.Pending == 0:
			if r.Rounds+1 >= domain.MaxRepairRounds+1 {
				return "blocked", map[string]any{"reason": fmt.Sprintf("checks failed after %d %s rounds", r.Rounds, r.Kind()), "failed": status.Failed}, nil
			}
			return "red", map[string]any{"failed": status.Failed, "passed": status.Passed}, nil
		case status.Pending == 0 && status.MergeState == "BEHIND" && !updated:
			if err := d.Forge.UpdateBranch(ctx, r.Repository, r.PullRequest); err != nil {
				if ctx.Err() != nil {
					return "", nil, err
				}
				return "blocked", map[string]any{"reason": "the branch is behind and could not be updated: " + bounded(err.Error(), 1000)}, nil
			}
			updated = true
		case status.Pending == 0 && status.MergeState == "DIRTY":
			return "blocked", map[string]any{"reason": "the pull request conflicts with its base"}, nil
		case status.Pending == 0 && len(status.Failed) == 0 && status.Passed > 0 && (status.MergeState == "CLEAN" || status.MergeState == "HAS_HOOKS" || status.MergeState == "UNSTABLE"):
			return "green", map[string]any{"passed": status.Passed, "polls": polls, "merge_state": status.MergeState}, nil
		case status.Pending == 0 && len(status.Failed) == 0 && status.Passed > 0 && status.MergeState == "BLOCKED":
			// Green checks but the forge still blocks the merge: a review
			// rule or protection the console cannot satisfy by waiting.
			if polls > 2 {
				return "blocked", map[string]any{"reason": "checks passed but the forge blocks the merge (review or protection rule)", "passed": status.Passed}, nil
			}
		}
		if !d.now().Before(deadline) {
			return "blocked", map[string]any{"reason": fmt.Sprintf("checks did not settle within %s", d.watchDeadline()), "pending": status.Pending}, nil
		}
		if err := d.sleep(ctx, d.poll()); err != nil {
			return "", nil, err
		}
	}
}

// automaticMerge reports whether decide records an automatic merge: only a
// repair under repair.auto_merge. axlr_improve has no such transition.
func (d *CeremonyDriver) automaticMerge(run domain.CeremonyRun) bool {
	return d.RepairPolicy.AutoMerge && run.Repair != nil && !run.Repair.Improvement
}

func (d *CeremonyDriver) watchDeadline() time.Duration {
	if d.RepairPolicy.WatchDeadline > 0 {
		return d.RepairPolicy.WatchDeadline
	}
	return repairWatch
}

func (d *CeremonyDriver) poll() time.Duration {
	if d.RepairPolicy.Poll > 0 {
		return d.RepairPolicy.Poll
	}
	return repairPoll
}

func (d *CeremonyDriver) sleep(ctx context.Context, wait time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, wait)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// maxTitleBytes bounds a pull request title, its ellipsis included.
const maxTitleBytes = 72

// proposalTitle is the pull request title: the ceremony's prefix and the
// first sentence of summary_en, which says what changed; before build or
// repair returned one, the title taken from the brief.
func proposalTitle(r domain.RepairRun) string {
	prefix := "Repair: "
	if r.Improvement {
		prefix = "Improve: "
	}
	if sentence := firstSentence(r.Summary); sentence != "" {
		return titleWithin(prefix, sentence)
	}
	return r.Title
}

// firstSentence is text up to its first sentence end (". ", "! ", "? " or a
// line break), without the closing mark.
func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	end := len(text)
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' || (text[i] == '.' || text[i] == '!' || text[i] == '?') && (i+1 == len(text) || text[i+1] == ' ') {
			end = i
			break
		}
	}
	return strings.TrimSpace(text[:end])
}

// titleWithin is prefix+text within maxTitleBytes: a longer one is cut at
// the last word boundary that leaves room for the ellipsis, or inside the
// word when the first word alone does not fit.
func titleWithin(prefix, text string) string {
	title := prefix + text
	if len(title) <= maxTitleBytes {
		return title
	}
	const ellipsis = "…"
	limit := maxTitleBytes - len(ellipsis)
	cut := strings.LastIndex(title[:limit+1], " ")
	if cut <= len(prefix) {
		for cut = limit; cut > 0 && !utf8Start(title[cut]); cut-- {
		}
	}
	return strings.TrimRight(title[:cut], " ,;:-") + ellipsis
}

// repairBody is the pull request description: what the ceremony knows, named
// as the console's work so nobody mistakes it for a hand-written change.
func repairBody(run domain.CeremonyRun) string {
	r := run.Repair
	if r.Improvement {
		return improveBody(run)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Repair made by the AXLR console through MADE ceremony `%s` %s, instance `%s`.\n\n", run.Definition, run.Version, run.Instance)
	if r.Cause != "" {
		body.WriteString("## Cause\n\n" + r.Cause + "\n\n")
	}
	if r.Fix != "" {
		body.WriteString("## Fix\n\n" + r.Fix + "\n\n")
	}
	if r.Summary != "" {
		body.WriteString("## Summary\n\n" + r.Summary + "\n\n")
	}
	if !run.Check.IsZero() {
		fmt.Fprintf(&body, "## Evidence\n\nCheck command `%s %s` failed before the repair and passed after it, run by the console.\n", run.Check.Program, strings.Join(run.Check.Args, " "))
	}
	if r.Rounds > 0 {
		fmt.Fprintf(&body, "\nCheck round %d after a red round: %s\n", r.Rounds+1, bounded(r.Feedback, 1000))
	}
	return body.String()
}

// awaitMerge parks the ceremony on decide for the person: the model is told to
// end its turn and the card opens.
func (d *CeremonyDriver) awaitMerge(run domain.CeremonyRun, report map[string]any) (StepResult, error) {
	r := run.Repair
	r.Awaiting, r.Decided, r.Granted = domain.AwaitingApproval, "", false
	command := "/repair"
	if r.Improvement {
		command = "/improve"
	}
	report["next_step"], report["instruction"] = run.Step, fmt.Sprintf("Pull request #%d (%s) is green and waits for the person's merge decision on the approval card (%s). Tell the user and end your turn.", r.PullRequest, r.URL, command)
	d.observe(run, "DECIDE", report, false, true)
	return accept(report, &run), nil
}

// ApproveMerge is the person's a on the repair card: record approve, grant the
// guard as the approver, apply merge_approved and let the console merge.
func (d *CeremonyDriver) approveMerge(ctx context.Context, s domain.Session, run domain.CeremonyRun) (StepResult, error) {
	r := run.Repair
	if d.Approver == nil {
		return StepResult{}, errors.New("the approver identity is not configured; prepare MADE: open /mcp, select MADE and press p")
	}
	if r.Decided == "decline" {
		return StepResult{}, errors.New("MADE already recorded the decline; the ceremony ends blocked")
	}
	keep := func(err error) (StepResult, error) { return StepResult{Run: &run}, err }
	if r.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "DECIDE", map[string]any{"decision": "approve", "pull_request": r.PullRequest, "head_sha": r.HeadSHA})
		if err != nil {
			return StepResult{}, err
		}
		r.Decided = decided
		if decided != "approve" {
			return keep(errors.New("MADE already recorded a different decision"))
		}
	}
	if !r.Granted {
		if err := d.Approver.ApproveGuard(ctx, run.Instance, "person_approves"); err != nil {
			return keep(fmt.Errorf("approve as the person: %w", err))
		}
		r.Granted = true
	}
	report := map[string]any{"step": run.Step, "decision": "approve", "pull_request": r.PullRequest}
	before := copyRun(run)
	r.Awaiting = ""
	return d.leaveApproval(ctx, s, run, before, "merge_approved", report)
}

// declineMerge is the person's d: the pull request stays open and the
// ceremony ends blocked with the reason on record.
func (d *CeremonyDriver) declineMerge(ctx context.Context, s domain.Session, run domain.CeremonyRun, reason string) (StepResult, error) {
	r := run.Repair
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return StepResult{}, errors.New("say why the merge is declined")
	case r.Decided == "approve":
		return StepResult{}, errors.New("MADE already recorded the approval; press a to finish it")
	}
	if r.Decided == "" {
		decided, err := d.completeDecision(ctx, run, "DECIDE", map[string]any{"decision": "decline", "reason": bounded(reason, 2000), "pull_request": r.PullRequest})
		if err != nil {
			return StepResult{}, err
		}
		r.Decided = decided
		if decided != "decline" {
			return StepResult{Run: &run}, errors.New("MADE already recorded the approval; press a to finish it")
		}
	}
	report := map[string]any{"step": run.Step, "decision": "decline", "reason": reason}
	before := copyRun(run)
	r.Awaiting = ""
	return d.leaveApproval(ctx, s, run, before, "merge_declined", report)
}

// Resume re-enters a repair whose console step was interrupted: a cancelled
// watch, a crash after propose, a reload. MADE says where the instance is; the
// pull request number comes back from the propose output it recorded.
func (d *CeremonyDriver) Resume(ctx context.Context, s domain.Session) (StepResult, bool, error) {
	run, live := s.Ceremony()
	if !live || run.Repair == nil || run.AwaitingPerson() || d == nil || d.Engine == nil || d.Forge == nil {
		return StepResult{}, false, nil
	}
	view, err := d.Engine.Inspect(ctx, run.Instance)
	if err != nil {
		return StepResult{}, false, fmt.Errorf("inspect %s: %w", run.Instance, err)
	}
	report := map[string]any{"resumed": run.Step, "state": view.State}
	hydrateRepair(&run, view)
	switch {
	case view.State == "COMPLETED" || view.State == "BLOCKED":
		result, err := d.enter(ctx, s, run, view.State, view.Outputs[run.Step], report)
		return result, true, err
	case len(view.Enabled) == 1:
		state, err := d.Engine.Transition(ctx, run.Instance, view.Enabled[0])
		if err != nil {
			return StepResult{}, false, err
		}
		result, err := d.enter(ctx, s, run, state, nil, report)
		return result, true, err
	}
	step, ok := stateSteps[view.State]
	if !ok || !repairConsoleSteps[step] {
		return StepResult{}, false, nil // a model step: the ordinary path reconciles it
	}
	if view.LiveErr != nil {
		return StepResult{}, false, fmt.Errorf("inspect %s: %w", run.Instance, view.LiveErr)
	}
	fence := view.Live[step]
	if fence == "" && !slices.Contains(view.Claimable, step) {
		return StepResult{}, false, nil
	}
	run.Step, run.Iteration = step, 1
	result, err := d.repairStepWith(ctx, s, run, report, fence)
	return result, true, err
}

// hydrateRepair takes the pull request MADE recorded at propose, so a console
// step re-entered after an interruption does not watch pull request 0.
func hydrateRepair(run *domain.CeremonyRun, view CeremonyView) {
	if run.Repair == nil {
		return
	}
	proposed, ok := view.Outputs["propose"]
	if !ok {
		return
	}
	if number, _ := proposed["pull_request"].(float64); number > 0 {
		run.Repair.PullRequest = int(number)
	}
	if url, _ := proposed["url"].(string); url != "" {
		run.Repair.URL = url
	}
	if sha, _ := proposed["head_sha"].(string); sha != "" {
		run.Repair.HeadSHA = sha
	}
}

// CanDecline reports whether the person may still decline the merge.
func CanDecline(run domain.CeremonyRun) bool {
	return run.Repair != nil && run.Repair.Decided != "approve"
}
