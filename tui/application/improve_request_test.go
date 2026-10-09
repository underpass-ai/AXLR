package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const validImprovementRequest = `{"description":"the agent cannot read the console's own log to explain a plugin that stops","expected":"a tool or documented path that shows the console's recent log lines","observed":"searched journalctl and the state directory by hand twice without finding the log","evidence":["journalctl shows python[3010] lines, not axlr-tui","the axlr data directory has no log file"],"tool_calls":["e1","e2"]}`

// frictionSession worked around a missing capability with two local calls.
func frictionSession(t *testing.T) domain.Session {
	t.Helper()
	return parentSession(t, activity{"e1", "local_exec", `{"program":"journalctl"}`, outcomeExitOne, false}, activity{"e2", "local_exec", `{"program":"ls"}`, outcomeExitZero, false})
}

func requestImprovement(t *testing.T, rig *repairRig, s domain.Session, args string) (map[string]any, error) {
	t.Helper()
	result, err := rig.repair.RequestImprovement(context.Background(), s, mustObject(t, args))
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(result)
	var out map[string]any
	_ = json.Unmarshal(encoded, &out)
	return out, nil
}

func TestRequestImprovementStartsAnImprovementSessionAndReportsTheMerge(t *testing.T) {
	bench := &happyWorkbench{}
	rig := newRepairRig(t, bench)
	parent := frictionSession(t)
	out, err := requestImprovement(t, rig, parent, validImprovementRequest)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := out["improvement"].(string)
	if out["accepted"] != true || !strings.HasPrefix(id, "20261005-1200-the-agent-cannot-read") || out["session"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !strings.Contains(out["instruction"].(string), "/improve") {
		t.Fatalf("result: %v", out)
	}
	record := waitStatus(t, rig.registry, id, domain.RepairAwaitingApproval)
	if !record.Improvement || record.Parent != parent.Export().ID {
		t.Fatalf("record: %+v", record)
	}
	request := rig.clones.requests[0]
	if request.Kind != ImproveKind || request.Slug != id || !strings.Contains(request.Brief, "Calls that showed the friction") || !strings.Contains(request.Brief, "feasible=false") || !strings.Contains(request.Brief, "journalctl") {
		t.Fatalf("clone request: %+v", request)
	}
	child, err := rig.store.Load(context.Background(), record.Session)
	if err != nil || child.Mode() != domain.ModeImprove {
		t.Fatalf("child session: %v %v", child.Mode(), err)
	}
	if err := rig.repair.Decide(context.Background(), id, true, ""); err != nil {
		t.Fatal(err)
	}
	record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if !strings.HasPrefix(record.Notice, "[AXLR] Self-improvement "+id+" merged pull request #7") || !strings.Contains(record.Notice, "does not contain the change") {
		t.Fatalf("notice: %s", record.Notice)
	}
	status, err := rig.repair.Status(context.Background(), parent, mustObject(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(status); !strings.Contains(string(encoded), `"kind":"improvement"`) {
		t.Fatalf("status: %s", encoded)
	}
	if _, err := requestImprovement(t, rig, parent, validImprovementRequest); err == nil || !strings.Contains(err.Error(), "at most one improvement per session") {
		t.Fatalf("second request from the same session: %v", err)
	}
}

func TestRequestImprovementRefusesWeakOrMisplacedRequests(t *testing.T) {
	plugin := parentSession(t, activity{"e1", "local_exec", `{}`, outcomeExitOne, false}, activity{"p1", "kmp_wake", `{}`, outcomePlugin, false})
	denied := parentSession(t, activity{"e1", "local_exec", `{}`, outcomeExitOne, false}, activity{"e2", "local_exec", `{}`, outcomeDenied, true})
	task := frictionSession(t)
	if err := task.SetMode(domain.ModeTask); err != nil {
		t.Fatal(err)
	}
	improve := frictionSession(t)
	if err := improve.SetMode(domain.ModeImprove); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		session domain.Session
		args    string
		want    string
	}{
		{"one call", frictionSession(t), strings.Replace(validImprovementRequest, `["e1","e2"]`, `["e1"]`, 1), "between 2 and 8"},
		{"unknown call", frictionSession(t), strings.Replace(validImprovementRequest, `["e1","e2"]`, `["e1","zz"]`, 1), "not in this session"},
		{"plugin call", plugin, strings.Replace(validImprovementRequest, `["e1","e2"]`, `["e1","p1"]`, 1), "MCP plugin tool"},
		{"denied call", denied, validImprovementRequest, "denied"},
		{"external cause", frictionSession(t), strings.Replace(validImprovementRequest, "by hand twice", "behind the proxy twice", 1), "external cause"},
		{"task session", task, validImprovementRequest, "plan and task sessions"},
		{"improvement session", improve, validImprovementRequest, "improvement session cannot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRepairRig(t, &happyWorkbench{})
			if _, err := requestImprovement(t, rig, tc.session, tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if len(rig.clones.requests) != 0 {
				t.Fatal("a refused request must not clone")
			}
		})
	}
	rig := newRepairRig(t, &happyWorkbench{})
	inClone, err := domain.NewSession("dddddddddddddddddddddddddddddddd", domain.Workspace(filepath.Join(rig.repairsIn, "x")), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rig.repair.RequestImprovement(context.Background(), inClone, mustObject(t, validImprovementRequest)); err == nil || !strings.Contains(err.Error(), "clone") {
		t.Fatalf("request from a clone: %v", err)
	}
}

func TestImprovementAdmissionBoundsHowOftenAgentsAct(t *testing.T) {
	r := &SelfRepair{Build: "0.4.2"}
	const session = domain.SessionID("p")
	for _, tc := range []struct {
		name    string
		records []domain.RepairRecord
		want    string
	}{
		{"repair session", []domain.RepairRecord{{ID: "r1", Session: session, Status: domain.RepairRunning}}, "repair session of r1"},
		{"shared slots", []domain.RepairRecord{{ID: "r1", Parent: "q", Session: "s1", Status: domain.RepairRunning}, {ID: "r2", Parent: "q", Session: "s2", Status: domain.RepairAwaitingMerge}}, "another self-repair or improvement is active (2 of 2 allowed by jobs.max_active)"},
		{"one per session", []domain.RepairRecord{{ID: "i1", Improvement: true, Parent: session, Session: "s1", Status: domain.RepairBlocked}}, "at most one improvement per session"},
		{"duplicate", []domain.RepairRecord{{ID: "i1", Improvement: true, Parent: "q", Session: "s1", Signature: "sig", Build: "0.4.2", Status: domain.RepairAwaitingMerge}}, "duplicate: improvement i1"},
		{"merged by this build", []domain.RepairRecord{{ID: "i1", Improvement: true, Parent: "q", Session: "s1", Signature: "sig", Build: "0.4.2", Status: domain.RepairCompleted, PullRequest: 9}}, "already improved: pull request #9"},
		{"cap per build", []domain.RepairRecord{
			{ID: "i1", Improvement: true, Parent: "a", Session: "s1", Build: "0.4.2", Status: domain.RepairCompleted},
			{ID: "i2", Improvement: true, Parent: "b", Session: "s2", Build: "0.4.2", Status: domain.RepairBlocked},
			{ID: "i3", Improvement: true, Parent: "c", Session: "s3", Build: "0.4.2", Status: domain.RepairFailed},
		}, "already started 3 improvements"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.admitImprovement(tc.records, session, "sig"); !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	older := []domain.RepairRecord{
		{ID: "i1", Improvement: true, Parent: "a", Session: "s1", Signature: "sig", Build: "0.4.1", Status: domain.RepairCompleted},
		{ID: "i2", Improvement: true, Parent: "b", Signature: "sig", Build: "0.4.2", Status: domain.RepairFailed},
		{ID: "r1", Parent: session, Session: "s3", Status: domain.RepairCompleted},
	}
	if got := r.admitImprovement(older, session, "sig"); got != "" {
		t.Fatalf("an older build's merge, a clone that failed and a finished repair must not block: %q", got)
	}
}

func TestImprovementToolIsOfferedWhereAnAgentMayAskForIt(t *testing.T) {
	snapshot := append(turnTools(), HostTools()...)
	for mode, offered := range map[domain.WorkMode]bool{domain.ModeNormal: true, domain.ModeWriter: true, domain.ModeRepair: false, domain.ModeImprove: false, domain.ModeTask: false, domain.ModePlan: false} {
		s := turnSession(t)
		if err := s.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		if got := hasDefinition(SessionTools(s, snapshot), HostRequestImprovementName); got != offered {
			t.Fatalf("%s: offered=%v", mode, got)
		}
	}
	id, _ := domain.NewHostToolIdentity(domain.HostOperationRequestImprovement)
	if !automaticallyApproves(nil, id) {
		t.Fatal("the request starts nothing the person has not agreed to; it needs no card")
	}
}
