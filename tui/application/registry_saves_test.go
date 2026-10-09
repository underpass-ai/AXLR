package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

var errDiskFull = errors.New("disk full")

// refusingPlans is a plan registry that refuses the saves refuse matches.
type refusingPlans struct {
	*memoryPlans
	refuse func(domain.PlanRecord) bool
}

func (f *refusingPlans) Save(ctx context.Context, record domain.PlanRecord) error {
	if f.refuse != nil && f.refuse(record) {
		return errDiskFull
	}
	return f.memoryPlans.Save(ctx, record)
}

// A plan change the registry refused is not announced as if it were saved,
// and the person learns of the refusal the next time the plans are listed.
func TestARunnerNeitherPublishesNorHidesAPlanChangeItCouldNotSave(t *testing.T) {
	plans := &refusingPlans{memoryPlans: &memoryPlans{}}
	_ = plans.Save(context.Background(), domain.PlanRecord{ID: "p-1", Session: "0123456789abcdef0123456789abcdef", Status: domain.PlanReady})
	runner := &PlanRunner{Plans: plans}
	events := runner.Events()
	plans.refuse = func(p domain.PlanRecord) bool { return p.Status == domain.PlanRunning }
	runner.update(context.Background(), "p-1", func(p *domain.PlanRecord) { p.Status = domain.PlanRunning })
	for len(events) > 0 {
		if event := <-events; event.Plan.Status == domain.PlanRunning {
			t.Fatal("a plan change the registry refused was published")
		}
	}
	records, err := runner.Records(context.Background())
	if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "p-1") {
		t.Fatalf("the refused save is not reported: %v", err)
	}
	if len(records) != 1 || records[0].Status != domain.PlanReady {
		t.Fatalf("records %+v", records)
	}
	if _, err := runner.Records(context.Background()); err != nil {
		t.Fatalf("a refusal is reported once: %v", err)
	}
}

// greenTask runs a task's red step and readies its green hand-back.
func greenTask(t *testing.T) (*CeremonyDriver, *refusingPlans, domain.Session) {
	t.Helper()
	d, files, checks, plans, s, _ := taskSetup(t, true)
	refusing := &refusingPlans{memoryPlans: plans}
	d.Plans = refusing
	files.files["lines_test.go"] = []byte("package textstat\nfunc TestLineCount(t *testing.T) { LineCount(\"\") }\n")
	checks.status = append(checks.status, "?? lines_test.go")
	checks.exits = []int{1}
	step(t, d, &s, `{"test_files":["lines_test.go"],"expected":"undefined: LineCount"}`)
	files.files["lines.go"] = []byte("package textstat\nfunc LineCount(s string) int { return 0 }\n")
	checks.status = append(checks.status, "?? lines.go")
	checks.exits = []int{0}
	return d, refusing, s
}

const greenArgs = `{"summary":"Added LineCount","summary_en":"LineCount counts lines."}`

// If the registry refuses a finished task's outcome, the task stays running
// there, and a restarted console resets it to pending and runs it again: the
// step must fail loudly instead of reporting DONE.
func TestATaskOutcomeTheRegistryRefusesFailsTheStep(t *testing.T) {
	d, plans, s := greenTask(t)
	plans.refuse = func(p domain.PlanRecord) bool { return p.Tasks[0].Status == domain.TaskDone }
	result, err := d.StepDone(context.Background(), s, mustObject(t, greenArgs))
	if err == nil || !strings.Contains(err.Error(), "disk full") || result.Accepted {
		t.Fatalf("an unsaved task outcome passed: %+v %v", result, err)
	}
}

// The hand-back's changed files and revision are evidence for the sync; when
// the registry refuses them the result says so.
func TestTaskHandbackSaysWhenTheRegistryRefusedItsChanges(t *testing.T) {
	d, plans, s := greenTask(t)
	plans.refuse = func(p domain.PlanRecord) bool {
		task := p.Tasks[0]
		return task.Status == domain.TaskRunning && task.Handback != nil && len(task.Handback.Changed) > 0
	}
	r := step(t, d, &s, greenArgs)
	if registry, _ := r["registry"].(string); r["ceremony"] != "DONE" || !strings.Contains(registry, "disk full") {
		t.Fatalf("hand-back: %v", r)
	}
}

type failingStarter struct{}

func (failingStarter) Start(context.Context, string) error {
	return errors.New("the workbench cannot open /w")
}

// A ready plan that could not start keeps its start error in the registry;
// when the registry refuses that too, the result still says why it did not
// start.
func TestAPlanThatCannotStartSaysSoWhenTheRegistryRefusesTheError(t *testing.T) {
	d, _, plans, _, s := planDriver(t)
	d.Plan.AutoApprove, d.Starter = true, failingStarter{}
	d.Plans = &refusingPlans{memoryPlans: plans, refuse: func(p domain.PlanRecord) bool { return p.Error != "" }}
	report := step(t, d, &s, planArgs(t, goodTasks))
	memory, _ := report["memory"].(string)
	if report["ceremony"] != "READY" || !strings.Contains(memory, "the workbench cannot open /w") || !strings.Contains(memory, "disk full") {
		t.Fatalf("plan start: %v", report)
	}
}

// A repair record the registry refused is neither announced nor lost: the
// error that ends the request names it, and an update that fails later is
// reported the next time the repairs are listed.
func TestSelfRepairNeitherPublishesNorHidesARecordItCouldNotSave(t *testing.T) {
	rig := newRepairRig(t, &happyWorkbench{})
	events := rig.repair.Events()
	rig.clones.err = errors.New("gh repo clone o/r: exit 4: gh: not logged in")
	rig.registry.refuse = func(r domain.RepairRecord) bool { return r.Status == domain.RepairFailed }
	_, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err == nil || !strings.Contains(err.Error(), "not logged in") || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("clone failure: %v", err)
	}
	for len(events) > 0 {
		if event := <-events; event.Record.Status == domain.RepairFailed {
			t.Fatal("a repair record the registry refused was published")
		}
	}
	saved := rig.registry.records[0]
	run := &repairRun{record: saved}
	rig.registry.refuse = func(r domain.RepairRecord) bool { return r.Status == domain.RepairCompleted }
	rig.repair.update(context.Background(), run, func(r *domain.RepairRecord) { r.Status = domain.RepairCompleted })
	for len(events) > 0 {
		if event := <-events; event.Record.Status == domain.RepairCompleted {
			t.Fatal("a repair update the registry refused was published")
		}
	}
	if _, err := rig.repair.Records(context.Background()); err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), saved.ID) {
		t.Fatalf("the refused update is not reported: %v", err)
	}
	if _, err := rig.repair.Records(context.Background()); err != nil {
		t.Fatalf("a refusal is reported once: %v", err)
	}
}
