package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// incidentEngine models axlr_incident 1.0 as the spike observed it in MADE
// 0.9.1: REVIEW repeats twice, the human guard persists for the instance,
// and only the current visit's outputs count.
type incidentEngine struct {
	state      string
	outputs    map[string]map[string]any
	rejections int
	guard      bool
	calls      []string
	keys       []string
	leases     map[string]time.Duration
	failNext   map[string]int
}

func newIncidentEngine() *incidentEngine {
	return &incidentEngine{outputs: map[string]map[string]any{}, leases: map[string]time.Duration{}, failNext: map[string]int{}}
}

func (e *incidentEngine) fail(op string) error {
	if e.failNext[op] > 0 {
		e.failNext[op]--
		return errors.New(op + " failed")
	}
	return nil
}

func (e *incidentEngine) Ready(context.Context, string, string) error { return nil }
func (e *incidentEngine) Start(context.Context, string, string, string, map[string]string) error {
	e.state = "TRIAGE"
	return nil
}
func (e *incidentEngine) Claim(_ context.Context, _, step, key string, lease time.Duration) (string, error) {
	if slices.Contains(e.keys, key) {
		return "", fmt.Errorf("claim key %s reused", key)
	}
	e.keys = append(e.keys, key)
	e.leases[step] = lease
	e.calls = append(e.calls, "claim "+step)
	return "fence-" + step, nil
}
func (e *incidentEngine) Complete(_ context.Context, _, step, _ string, output map[string]any) error {
	if err := e.fail("complete " + step); err != nil {
		return err
	}
	e.calls = append(e.calls, "complete "+step)
	e.outputs[step] = output
	if step == "review" && output["accepted"] != true {
		e.rejections++
	}
	return nil
}
func (e *incidentEngine) enabled() []string {
	done := func(step string) bool { return e.outputs[step] != nil }
	var out []string
	switch e.state {
	case "TRIAGE", "TIMELINE", "ANALYSIS", "PUBLISH":
		step := map[string]string{"TRIAGE": "triage", "TIMELINE": "timeline", "ANALYSIS": "analysis", "PUBLISH": "publish"}[e.state]
		if done(step) {
			out = append(out, map[string]string{"triage": "triaged", "timeline": "timelined", "analysis": "analyzed", "publish": "published"}[step])
		}
	case "REVIEW":
		if done("review") && e.outputs["review"]["accepted"] == true {
			out = append(out, "reviewed")
		}
		if e.rejections >= 2 {
			out = append(out, "review_exhausted")
		}
	case "APPROVAL":
		if done("present") && e.guard {
			out = append(out, "approved")
		}
		if done("present") && e.outputs["present"]["decision"] == "return" {
			out = append(out, "returned")
		}
	}
	return out
}
func (e *incidentEngine) Transition(_ context.Context, _, trigger string) (string, error) {
	e.calls = append(e.calls, "transition "+trigger)
	if err := e.fail("transition " + trigger); err != nil {
		return "", err
	}
	if !slices.Contains(e.enabled(), trigger) {
		return "", fmt.Errorf("%s is not enabled in %s", trigger, e.state)
	}
	e.state = map[string]string{"triaged": "TIMELINE", "timelined": "ANALYSIS", "analyzed": "REVIEW", "reviewed": "APPROVAL", "review_exhausted": "BLOCKED", "approved": "PUBLISH", "returned": "REVIEW", "published": "COMPLETED"}[trigger]
	if e.state == "REVIEW" {
		e.rejections = 0
		delete(e.outputs, "revise")
		delete(e.outputs, "review")
	}
	delete(e.outputs, "present")
	return e.state, nil
}
func (e *incidentEngine) Inspect(context.Context, string) (CeremonyView, error) {
	view := CeremonyView{State: e.state, Enabled: e.enabled()}
	if e.state == "REVIEW" && e.rejections > 0 && e.rejections < 2 && e.outputs["review"]["accepted"] != true {
		view.Claimable = []string{"revise"}
	}
	if e.state == "REVIEW" && e.outputs["revise"] != nil && e.outputs["review"] == nil {
		view.Claimable = []string{"review"}
	}
	return view, nil
}

type fakeFiles struct{ files map[string][]byte }

func (f *fakeFiles) Read(_ context.Context, path string, max int) ([]byte, bool, error) {
	content, ok := f.files[path]
	if len(content) > max {
		content = content[:max]
	}
	return content, ok, nil
}
func (f *fakeFiles) Write(_ context.Context, path string, content []byte) error {
	f.files[path] = append([]byte(nil), content...)
	return nil
}
func (f *fakeFiles) MakeDir(context.Context, string) error { return nil }

type fakeReviewer struct {
	verdicts []ReviewVerdict
	errs     []error
	requests []ReviewRequest
}

func (f *fakeReviewer) Review(ctx context.Context, request ReviewRequest) (ReviewVerdict, error) {
	f.requests = append(f.requests, request)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return ReviewVerdict{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ReviewVerdict{}, err
	}
	verdict := f.verdicts[0]
	f.verdicts = f.verdicts[1:]
	verdict.Model = "reviewer-model"
	return verdict, nil
}

type fakeApprover struct {
	fails  int
	grants []string
	engine *incidentEngine
}

func (f *fakeApprover) ApproveGuard(_ context.Context, _, guard string) error {
	if f.fails > 0 {
		f.fails--
		return errors.New("approver unreachable")
	}
	f.grants = append(f.grants, guard)
	f.engine.guard = true
	return nil
}

type labelMemory struct {
	labels  map[string][]string
	summary string
}

func (m *labelMemory) Wake(context.Context, string) (string, error) { return "", nil }
func (m *labelMemory) Record(_ context.Context, _ string, labels map[string][]string, _, summary, _ string) error {
	m.labels, m.summary = labels, summary
	return nil
}

type incidentRig struct {
	engine   *incidentEngine
	files    *fakeFiles
	reviewer *fakeReviewer
	approver *fakeApprover
	memory   *labelMemory
	driver   *CeremonyDriver
	session  domain.Session
}

func newIncidentRig(t *testing.T, verdicts ...bool) *incidentRig {
	t.Helper()
	engine := newIncidentEngine()
	r := &incidentRig{engine: engine, files: &fakeFiles{files: map[string][]byte{"logs/api.log": []byte("500 at 10:02")}}, reviewer: &fakeReviewer{}, approver: &fakeApprover{engine: engine}, memory: &labelMemory{}}
	for _, accepted := range verdicts {
		verdict := ReviewVerdict{Accepted: accepted}
		if !accepted {
			verdict.Findings = []string{"the timeline blames the on-call engineer"}
		}
		r.reviewer.verdicts = append(r.reviewer.verdicts, verdict)
	}
	r.driver = &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Memory: r.memory, Files: r.files, Reviewer: r.reviewer, Approver: r.approver, Now: func() time.Time { return time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC) }}
	r.session = turnSession(t)
	if err := r.session.SetMode(domain.ModeIncident); err != nil {
		t.Fatal(err)
	}
	if err := r.driver.Begin(context.Background(), &r.session, "checkout returned 500s for 40 minutes"); err != nil {
		t.Fatal(err)
	}
	return r
}

const (
	triageArgs   = `{"summary":"Checkout failed","impact":"12% of orders for 40 min","severity":"sev2","detection":"5xx alert","service":"checkout","slug":"checkout-500s"}`
	timelineArgs = `{"timeline":[{"at":"2026-10-01T10:00:00Z","event":"deploy","evidence":"deploy log in CI"},{"at":"2026-10-01T10:02:00Z","event":"500s start","evidence":"logs/api.log#L1"}]}`
	analysisArgs = `{"root_cause":"pool size","contributing_factors":["no canary"],"went_well":["alert fired"],"went_badly":["slow rollback"],"actions":[{"title":"canary","kind":"preventive","owner":"platform","due":"2026-11-01","verification":"canary blocks bad deploy in staging"}]}`
	reviseArgs   = `{"draft_path":"docs/incidents/checkout-500s.draft.md"}`
)

func (r *incidentRig) toReview(t *testing.T) {
	t.Helper()
	for _, args := range []string{triageArgs, timelineArgs, analysisArgs} {
		if report := step(t, r.driver, &r.session, args); report["accepted"] != true {
			t.Fatalf("%s: %v", args, report)
		}
	}
	r.writeDraft("# Postmortem v1")
}

func (r *incidentRig) writeDraft(text string) {
	r.files.files["docs/incidents/checkout-500s.draft.md"] = []byte(text)
}

func (r *incidentRig) run(t *testing.T) domain.CeremonyRun {
	t.Helper()
	run, live := r.session.Ceremony()
	if !live {
		t.Fatal("ceremony is not live")
	}
	return run
}

// apply mirrors the console: it keeps whatever run the driver hands back.
func (r *incidentRig) apply(result StepResult, err error) error {
	switch {
	case result.Accepted && result.Run == nil:
		r.session.FinishCeremony()
	case result.Run != nil:
		if setErr := r.session.SetCeremony(*result.Run); setErr != nil {
			return setErr
		}
	}
	return err
}

func TestIncidentRunsToPublicationThroughAReviewRoundAndThePerson(t *testing.T) {
	r := newIncidentRig(t, false, true)
	r.toReview(t)
	if report := step(t, r.driver, &r.session, reviseArgs); report["next_step"] != "revise" {
		t.Fatalf("rejected review should send the draft back: %v", report)
	}
	run := r.run(t)
	if run.Iteration != 2 || !strings.Contains(Instruction(run), "blames the on-call engineer") {
		t.Fatalf("second round lacks the findings: %d %s", run.Iteration, Instruction(run))
	}
	r.writeDraft("# Postmortem v2")
	if report := step(t, r.driver, &r.session, reviseArgs); report["next_step"] != "present" {
		t.Fatalf("accepted review should reach the person: %v", report)
	}
	if r.engine.leases["present"] != presentLease {
		t.Fatalf("present lease %v", r.engine.leases["present"])
	}
	draft, intact, err := r.driver.Draft(context.Background(), r.session)
	if err != nil || !intact || draft != "# Postmortem v2" {
		t.Fatalf("approval card draft %q %v %v", draft, intact, err)
	}
	if err := r.apply(r.driver.Approve(context.Background(), r.session)); err != nil {
		t.Fatal(err)
	}
	run = r.run(t)
	if run.Step != "publish" || run.AwaitingPerson() {
		t.Fatalf("approval did not reach publish: %+v", run)
	}
	if got := string(r.files.files["docs/incidents/2026-10-02-checkout-500s.md"]); got != "# Postmortem v2" {
		t.Fatalf("published bytes %q", got)
	}
	if report := step(t, r.driver, &r.session, `{"report":"publicado","summary_en":"Checkout 500s from pool size; canary action owned by platform."}`); report["ceremony"] != "COMPLETED" {
		t.Fatalf("publish: %v", report)
	}
	for key, want := range map[string]string{"incident": "checkout-500s", "service": "checkout", "severity": "sev2"} {
		if got := r.memory.labels[key]; len(got) != 1 || got[0] != want {
			t.Fatalf("label %s = %v", key, got)
		}
	}
	if review := r.engine.outputs["review"]; review["review_mode"] != "independent_context" || review["reviewer_model"] != "reviewer-model" {
		t.Fatalf("review output %v", review)
	}
}

func TestPresentOrderCompletesBeforeTheGuardAndApproveResumesAfterAFailedGrant(t *testing.T) {
	r := newIncidentRig(t, true)
	r.toReview(t)
	step(t, r.driver, &r.session, reviseArgs)
	r.approver.fails = 1
	if err := r.apply(r.driver.Approve(context.Background(), r.session)); err == nil {
		t.Fatal("a failed grant should be reported")
	}
	run := r.run(t)
	if run.Incident.Decided != "approve" || run.Incident.Granted || !run.AwaitingPerson() {
		t.Fatalf("after a failed grant: %+v", run.Incident)
	}
	if !slices.Contains(r.engine.calls, "complete present") || r.engine.guard {
		t.Fatalf("present must be completed and nothing approved: %v", r.engine.calls)
	}
	if CanReturn(run) {
		t.Fatal("d must be hidden once MADE recorded the approval")
	}
	completes := strings.Count(strings.Join(r.engine.calls, ","), "complete present")
	if err := r.apply(r.driver.Approve(context.Background(), r.session)); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(r.engine.calls, ","), "complete present") != completes {
		t.Fatal("a retried a completed present twice")
	}
	if run := r.run(t); run.Step != "publish" {
		t.Fatalf("retry did not finish the approval: %+v", run)
	}
}

func TestTheDraftGoesBackTwiceAtMost(t *testing.T) {
	r := newIncidentRig(t, true, true, true)
	r.toReview(t)
	step(t, r.driver, &r.session, reviseArgs)
	for n := 1; n <= domain.MaxIncidentReturns; n++ {
		if !CanReturn(r.run(t)) {
			t.Fatalf("return %d hidden", n)
		}
		if err := r.apply(r.driver.Return(context.Background(), r.session, "falta la acción de alertas")); err != nil {
			t.Fatal(err)
		}
		run := r.run(t)
		if run.Step != "revise" || run.Incident.Returns != n || !strings.Contains(Instruction(run), "falta la acción de alertas") {
			t.Fatalf("return %d: %+v", n, run.Incident)
		}
		if report := step(t, r.driver, &r.session, reviseArgs); report["next_step"] != "present" {
			t.Fatalf("re-review %d: %v", n, report)
		}
	}
	if CanReturn(r.run(t)) {
		t.Fatal("d offered a third time")
	}
	if _, err := r.driver.Return(context.Background(), r.session, "otra vez"); err == nil {
		t.Fatal("third return accepted")
	}
	if r.reviewer.requests[1].ReturnReason != "falta la acción de alertas" {
		t.Fatalf("reviewer did not see the person's reason: %+v", r.reviewer.requests[1])
	}
}

func TestStepDoneIsRefusedAndHiddenWhileThePersonDecides(t *testing.T) {
	r := newIncidentRig(t, true)
	r.toReview(t)
	step(t, r.driver, &r.session, reviseArgs)
	before := len(r.engine.calls)
	result, err := r.driver.StepDone(context.Background(), r.session, mustObject(t, `{"report":"x","summary_en":"y"}`))
	if err != nil || result.Accepted || !strings.Contains(string(result.Outcome.Content), "with the person") {
		t.Fatalf("step_done while awaiting: %+v %v", result, err)
	}
	if len(r.engine.calls) != before {
		t.Fatal("refusal reached MADE")
	}
	for _, tool := range SessionTools(r.session, HostTools()) {
		if tool.Name == HostStepDoneName {
			t.Fatal("axlr_step_done offered while the person decides")
		}
	}
}

func TestACancelledReviewLeavesReviseOpenAndReviewUnclaimed(t *testing.T) {
	r := newIncidentRig(t, true)
	r.toReview(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.driver.StepDone(ctx, r.session, mustObject(t, reviseArgs)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled review: %v", err)
	}
	for _, call := range r.engine.calls {
		if call == "complete revise" || call == "claim review" {
			t.Fatalf("cancelled review touched MADE: %v", r.engine.calls)
		}
	}
}

func TestReviewerFailuresAreCountedAndStopTheModel(t *testing.T) {
	r := newIncidentRig(t)
	r.reviewer.errs = []error{errors.New("bad verdict"), errors.New("bad verdict")}
	r.toReview(t)
	first, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, reviseArgs))
	if first.Accepted || first.Run == nil || first.Run.Incident.ReviewFailures != 1 {
		t.Fatalf("first failure: %+v", first)
	}
	_ = r.apply(first, nil)
	second, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, reviseArgs))
	if !strings.Contains(string(second.Outcome.Content), "end your turn") {
		t.Fatalf("second failure should stop the model: %s", second.Outcome.Content)
	}
}

func TestExhaustedReviewEndsBlocked(t *testing.T) {
	r := newIncidentRig(t, false, false)
	r.toReview(t)
	step(t, r.driver, &r.session, reviseArgs)
	if report := step(t, r.driver, &r.session, reviseArgs); report["ceremony"] != "BLOCKED" {
		t.Fatalf("two rejections: %v", report)
	}
}

func TestIncidentStepChecks(t *testing.T) {
	r := newIncidentRig(t)
	for _, args := range []string{
		`{"summary":"a","impact":"b","severity":"high","detection":"c","service":"d","slug":"e"}`,
		`{"summary":"a","impact":"b","severity":"sev1","detection":"c","service":"d","slug":"Not Kebab"}`,
	} {
		if result, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, args)); result.Accepted {
			t.Fatalf("triage accepted %s", args)
		}
	}
	step(t, r.driver, &r.session, triageArgs)
	for _, args := range []string{
		`{"timeline":[{"at":"2026-10-01T10:00:00Z","event":"x","evidence":"logs/missing.log"}]}`,
		`{"timeline":[{"at":"2026-10-01T11:00:00Z","event":"x","evidence":"y z"},{"at":"2026-10-01T10:00:00Z","event":"x","evidence":"y z"}]}`,
		`{"timeline":[{"at":"10:00","event":"x","evidence":"y z"}]}`,
	} {
		if result, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, args)); result.Accepted {
			t.Fatalf("timeline accepted %s", args)
		}
	}
	step(t, r.driver, &r.session, timelineArgs)
	past := strings.Replace(analysisArgs, "2026-11-01", "2026-10-01", 1)
	if result, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, past)); result.Accepted {
		t.Fatal("analysis accepted a past due date")
	}
	step(t, r.driver, &r.session, analysisArgs)
	for _, path := range []string{"docs/incidents/../../etc/x.draft.md", "notes/x.draft.md", "docs/incidents/x.md"} {
		r.files.files[path] = []byte("x")
		if result, _ := r.driver.StepDone(context.Background(), r.session, mustObject(t, `{"draft_path":"`+path+`"}`)); result.Accepted {
			t.Fatalf("draft_path %s accepted", path)
		}
	}
}

func TestApprovalRefusesADraftChangedAfterReview(t *testing.T) {
	r := newIncidentRig(t, true)
	r.toReview(t)
	step(t, r.driver, &r.session, reviseArgs)
	r.writeDraft("# edited after review")
	if _, intact, _ := r.driver.Draft(context.Background(), r.session); intact {
		t.Fatal("card should flag the changed draft")
	}
	if _, err := r.driver.Approve(context.Background(), r.session); err == nil || r.engine.outputs["present"] != nil {
		t.Fatalf("approved a changed draft: %v", err)
	}
}

func TestAResumeRunsTheReviewThatNeverRan(t *testing.T) {
	r := newIncidentRig(t, true)
	r.toReview(t)
	r.engine.failNext["claim review"] = 0
	// The review's claim failed after revise landed: the console crashed.
	run := r.run(t)
	run.Incident.DraftPath, run.Incident.DraftDigest = "docs/incidents/checkout-500s.draft.md", digestOf([]byte("# Postmortem v1"))
	r.engine.outputs["revise"] = map[string]any{"draft_path": run.Incident.DraftPath}
	result, err := r.driver.reconcile(context.Background(), r.session, run, nil, map[string]any{}, errors.New("claim review: lost"))
	if err != nil || result.Run == nil || result.Run.Step != "present" {
		t.Fatalf("resume did not review: %+v %v", result, err)
	}
}
