package application

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Every turn claim reads its operation id and reports running; the
// completion reports finished; the sidecar keeps what the reports need.
func TestSessionCeremonyReportsEachTurnToMADE(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	c.Host = HostIdentity{Owner: "w", AgentID: "install", Incarnation: "run"}
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.OperationID != "op-fence-turn" || instance.LeaseMS != sessionLease.Milliseconds() || instance.ClaimedAt.IsZero() || instance.Reports != 1 || instance.Reported.IsZero() {
		t.Fatalf("instance %+v", instance)
	}
	if len(engine.reports) != 1 || engine.reports[0].Status != "running" || engine.reports[0].Execution != string(s.ID()) || engine.reports[0].Role != "AXLR" || engine.reports[0].Host.Incarnation != "run" || !strings.Contains(engine.reports[0].Summary, "turn 1 of session") || engine.leases[0] != sessionLease {
		t.Fatalf("reports %+v leases %v", engine.reports, engine.leases)
	}
	if err := s.CompleteAssistant(assistant("hola")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ = s.Instance()
	if len(engine.reports) != 2 || engine.reports[1].Status != "finished" || engine.reports[1].Sequence != 2 || instance.OperationID != "" || instance.Fence != "" {
		t.Fatalf("reports %+v instance %+v", engine.reports, instance)
	}
	// A report MADE refuses is logged and changes nothing else.
	var logged []string
	c.Log = func(line string) { logged = append(logged, line) }
	engine.failReports = true
	if err := s.BeginTurn("otra", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if instance, _ = s.Instance(); instance.Fence == "" || instance.Reported.IsZero() == false || len(logged) == 0 || !strings.Contains(logged[len(logged)-1], "report running") {
		t.Fatalf("instance %+v logged %v", instance, logged)
	}
	// Without an operation id nothing is reported.
	engine.noOperation, engine.failReports = true, false
	other := turnSession(t)
	if err := other.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	before := len(engine.reports)
	c.BeginTurn(context.Background(), &other)
	if instance, _ = other.Instance(); instance.Fence == "" || len(engine.reports) != before {
		t.Fatalf("instance %+v reports %d", instance, len(engine.reports))
	}
}

// A long turn reports every five minutes and renews its lease at three
// quarters; a short one does neither.
func TestHeartbeatReportsAndRenewsALongTurn(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	clock := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return clock }
	s := turnSession(t)
	if err := s.BeginTurn("trabajo largo", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	c.Heartbeat(context.Background(), &s)
	if len(engine.reports) != 1 {
		t.Fatalf("a fresh claim reported again: %+v", engine.reports)
	}
	clock = clock.Add(hostHeartbeat)
	c.Heartbeat(context.Background(), &s)
	if len(engine.reports) != 2 || engine.reports[1].Activity != "heartbeat" || engine.reports[1].Status != "running" {
		t.Fatalf("%+v", engine.reports)
	}
	instance, _ := s.Instance()
	if !instance.Reported.Equal(clock) || len(engine.leases) != 1 {
		t.Fatalf("instance %+v leases %v", instance, engine.leases)
	}
	clock = clock.Add(sessionLease*3/4 - hostHeartbeat + time.Second)
	c.Heartbeat(context.Background(), &s)
	if len(engine.leases) != 2 || engine.leases[1] != sessionLease || !strings.Contains(strings.Join(engine.calls, ";"), "renew turn") {
		t.Fatalf("leases %v calls %v", engine.leases, engine.calls)
	}
	if instance, _ = s.Instance(); !instance.ClaimedAt.Equal(clock) {
		t.Fatalf("instance %+v", instance)
	}
	// Between turns there is nothing to report.
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	_ = c.EndTurn(context.Background(), &s)
	reports := len(engine.reports)
	clock = clock.Add(time.Hour)
	c.Heartbeat(context.Background(), &s)
	if len(engine.reports) != reports {
		t.Fatalf("reported between turns: %+v", engine.reports)
	}
	var none *SessionCeremony
	none.Heartbeat(context.Background(), &s)
}

// A driven ceremony reports running at its first claim, finished at each
// hand-back and blocked at a stall.
func TestCeremonyDriverReportsItsSteps(t *testing.T) {
	engine := &fakeEngine{}
	d := CeremonyDriver{Engine: engine, Checks: &fakeChecks{exits: []int{1}}, Host: HostIdentity{Owner: "w"}, Roles: func(definition, version string) string { return "DRIVER" }}
	s := debugSession(t)
	if err := d.Begin(context.Background(), &s, "falla"); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Ceremony()
	if run.OperationID != "op-fence-reproduce" || run.Reports != 1 || len(engine.reports) != 1 || engine.reports[0].Status != "running" || engine.reports[0].Role != "DRIVER" || engine.reports[0].Execution != run.Instance {
		t.Fatalf("run %+v reports %+v", run, engine.reports)
	}
	if err := s.BeginTurn("falla", turnTools()); err != nil {
		t.Fatal(err)
	}
	result, err := d.StepDone(context.Background(), s, mustObject(t, `{"check_command":{"program":"false"},"expected":"0","observed":"1"}`))
	if err != nil || !result.Accepted {
		t.Fatalf("%+v %v", result, err)
	}
	if len(engine.reports) < 2 || engine.reports[1].Status != "finished" || engine.reports[1].Step != "reproduce" {
		t.Fatalf("%+v", engine.reports)
	}
	if err := s.SetCeremony(*result.Run); err != nil {
		t.Fatal(err)
	}
	if err := d.StopCeremony(context.Background(), &s, "stalled"); err != nil {
		t.Fatal(err)
	}
	last := engine.reports[len(engine.reports)-1]
	if last.Status != "blocked" || last.Activity != "stall" || last.Step != "diagnose" {
		t.Fatalf("%+v", last)
	}
}

// A claim another incarnation of this install left open is declared lost
// before it is completed as interrupted; one's own claim is not.
func TestInterruptedClaimOfAnotherIncarnationIsHandedOff(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	c.Host = HostIdentity{Owner: "w", AgentID: "install", Incarnation: "run2", Hostname: "gx10"}
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	instance.ClaimIncarnation = "install:run1"
	instance.Reported = time.Date(2026, 10, 13, 17, 0, 0, 0, time.UTC)
	if err := s.SetInstance(instance); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("segundo", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if len(engine.handoffs) != 1 || engine.handoffs[0].Incarnation != "install:run1" || engine.handoffs[0].Fence != "fence-turn" || !strings.Contains(engine.handoffs[0].Evidence, "run install:run1 on host gx10") || !strings.Contains(engine.handoffs[0].Evidence, "last reported at 2026-10-13T17:00:00Z") || !strings.Contains(engine.handoffs[0].Evidence, "run install:run2 found") {
		t.Fatalf("%+v", engine.handoffs)
	}
	if calls := strings.Join(engine.calls, ";"); !strings.Contains(calls, "handoff install:run1;report finished turn;complete turn") {
		t.Fatalf("calls %v", engine.calls)
	}
	// The same incarnation's own interrupted claim needs no handoff.
	if err := s.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("tercero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if len(engine.handoffs) != 1 {
		t.Fatalf("%+v", engine.handoffs)
	}
}

// The close leaves one session_closed entry under the session's about.
func TestCloseWritesASessionClosedEntry(t *testing.T) {
	engine := &fakeEngine{}
	c, _, journal, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if err := s.CompleteAssistant(assistant("hola")); err != nil {
		t.Fatal(err)
	}
	_ = c.EndTurn(context.Background(), &s)
	if err := c.Close(context.Background(), &s, "exit"); err != nil {
		t.Fatal(err)
	}
	id := string(s.ID())
	if len(journal.entries) != 1 || journal.entries[0].Kind != "observation" || journal.entries[0].Labels["event"][0] != "closed" || journal.entries[0].About != "ws:"+id || journal.entries[0].Key != "axlr:closed:"+id || journal.entries[0].Summary != "Session closed after 1 turn (exit)." {
		t.Fatalf("%+v", journal.entries)
	}
}
