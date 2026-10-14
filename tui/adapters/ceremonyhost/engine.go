package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const (
	actorID    = "axlr"
	actorKind  = "agent"
	claimLease = 3600000 // a model step can take many minutes
)

// Engine is application.CeremonyEnginePort over the connected MADE plugin.
// Owner is the lease owner of every claim: the console's MADE work
// identity, since MADE accepts a status report only from the claim's owner
// (13 October 2026); "" keeps the historical "axlr".
type Engine struct {
	Tools application.ToolExecutionPort
	Owner string
}

func (e Engine) owner() string {
	if e.Owner != "" {
		return e.Owner
	}
	return actorID
}

var _ application.CeremonyEnginePort = Engine{}

func (e Engine) made(ctx context.Context, tool string, arguments map[string]any) (map[string]any, error) {
	return callPlugin(ctx, e.Tools, "made", tool, arguments)
}

func (e Engine) Ready(ctx context.Context, definition, version string) error {
	pin, ok := pinned(definition, version)
	if !ok {
		return fmt.Errorf("%s %s is not a definition AXLR ships", definition, version)
	}
	published, err := e.made(ctx, "made_get_ceremony_definition", map[string]any{"ceremony": definition, "version": version})
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) && (refused.Code == "not_found" || strings.Contains(refused.Message, "not found")) {
			return application.NotPreparedError(application.MissingDefinition{Name: definition, Version: version})
		}
		return err
	}
	if digest, _ := published["digest"].(string); digest != pin.Digest {
		return fmt.Errorf("published %s %s differs from the definition AXLR ships (digest %s)", definition, version, digest)
	}
	return nil
}

func (e Engine) Start(ctx context.Context, definition, version, instance string, inputs map[string]string) error {
	fields := map[string]any{}
	for key, value := range inputs {
		fields[key] = value
	}
	_, err := e.made(ctx, "made_start_published_ceremony", map[string]any{"ceremony": definition, "version": version, "ceremony_id": instance, "actor_id": actorID, "actor_kind": actorKind, "context": fields})
	return err
}

func (e Engine) Claim(ctx context.Context, instance, step, key string, lease time.Duration) (string, error) {
	ttl := int64(claimLease)
	if lease > 0 {
		ttl = lease.Milliseconds()
	}
	claim, err := e.made(ctx, "made_claim_ceremony_step", map[string]any{"ceremony_id": instance, "step_id": step, "actor_kind": actorKind, "lease_owner_id": e.owner(), "idempotency_key": key, "lease_ttl_ms": ttl})
	if err != nil {
		return "", err
	}
	fence, _ := claim["claim_fence"].(string)
	if fence == "" {
		return "", errors.New("MADE returned no claim fence")
	}
	return fence, nil
}

func (e Engine) Complete(ctx context.Context, instance, step, fence string, output map[string]any) error {
	_, err := e.made(ctx, "made_complete_ceremony_step", map[string]any{"ceremony_id": instance, "step_id": step, "actor_kind": actorKind, "claim_fence": fence, "status": "completed", "output": output})
	return err
}

func (e Engine) Inspect(ctx context.Context, instance string) (application.CeremonyView, error) {
	current, err := e.made(ctx, "made_get_ceremony_instance", map[string]any{"ceremony_id": instance})
	if err != nil {
		return application.CeremonyView{}, err
	}
	view := application.CeremonyView{}
	view.State, _ = current["current_state"].(string)
	if view.State == "" {
		return application.CeremonyView{}, errors.New("MADE returned no current state")
	}
	transitions, _ := current["transitions"].([]any)
	for _, raw := range transitions {
		transition, _ := raw.(map[string]any)
		if enabled, _ := transition["enabled"].(bool); enabled {
			if trigger, _ := transition["trigger"].(string); trigger != "" {
				view.Enabled = append(view.Enabled, trigger)
			}
		}
	}
	visit, _ := current["current_state_visit"].(float64)
	steps, _ := current["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		id, _ := step["step_id"].(string)
		state, _ := step["state_id"].(string)
		status, _ := step["status"].(string)
		stepVisit, _ := step["state_visit"].(float64)
		if id == "" || status != "completed" {
			continue
		}
		output, _ := step["output"].(map[string]any)
		if output == nil {
			output = map[string]any{}
		}
		if view.Outputs == nil {
			view.Outputs = map[string]map[string]any{}
		}
		view.Outputs[id] = output // steps are listed in order, so the latest visit wins
		if state != view.State || stepVisit != visit {
			continue
		}
		if view.Completed == nil {
			view.Completed = map[string]map[string]any{}
		}
		view.Completed[id] = output
	}
	claimable, _ := current["claimable_step_ids"].([]any)
	for _, raw := range claimable {
		if step, _ := raw.(string); step != "" {
			view.Claimable = append(view.Claimable, step)
		}
	}
	// A live claim of ours survives a crash; its fence is the only way to
	// finish that step before the lease ends (recovery path
	// complete_with_original_fence). When MADE cannot say, the view says
	// so instead of reporting no live claim.
	if resume, err := e.made(ctx, "made_inspect_ceremony_resume", map[string]any{"ceremony_id": instance}); err != nil {
		view.LiveErr = fmt.Errorf("live claims of %s unknown: %w", instance, err)
	} else {
		claims, _ := resume["claims"].([]any)
		for _, raw := range claims {
			claim, _ := raw.(map[string]any)
			step, _ := claim["step_id"].(string)
			fence, _ := claim["claim_fence"].(string)
			phase, _ := claim["phase"].(string)
			owner, _ := claim["owner"].(string)
			if step != "" && fence != "" && phase == "live" && (owner == e.owner() || owner == actorID) {
				if view.Live == nil {
					view.Live = map[string]string{}
				}
				view.Live[step] = fence
			}
		}
	}
	return view, nil
}

func (e Engine) Cancel(ctx context.Context, instance, reason string) error {
	_, err := e.made(ctx, "made_cancel_ceremony", map[string]any{"ceremony_id": instance, "actor_id": actorID, "actor_kind": actorKind, "reason": reason})
	return err
}

func (e Engine) Transition(ctx context.Context, instance, trigger string) (string, error) {
	if _, err := e.made(ctx, "made_apply_ceremony_transition", map[string]any{"ceremony_id": instance, "trigger": trigger, "actor_kind": actorKind}); err != nil {
		return "", err
	}
	current, err := e.made(ctx, "made_get_ceremony_instance", map[string]any{"ceremony_id": instance})
	if err != nil {
		return "", err
	}
	state, _ := current["current_state"].(string)
	if state == "" {
		return "", errors.New("MADE returned no current state")
	}
	return state, nil
}

// Operation reads what MADE checks a status report against besides the
// fence and the owner: the claim's operation id, which
// made_inspect_ceremony_resume carries and the claim receipt does not, and
// the attempt of the claimed step record, which only the instance's step
// list carries. MADE 0.10.0 reads a report without an attempt as attempt
// 0 and refuses it (14 October 2026).
func (e Engine) Operation(ctx context.Context, instance, step, fence string) (application.ClaimOperation, error) {
	resume, err := e.made(ctx, "made_inspect_ceremony_resume", map[string]any{"ceremony_id": instance})
	if err != nil {
		return application.ClaimOperation{}, err
	}
	operation := application.ClaimOperation{}
	claims, _ := resume["claims"].([]any)
	for _, raw := range claims {
		claim, _ := raw.(map[string]any)
		if claim["claim_fence"] == fence && claim["step_id"] == step {
			if id, _ := claim["operation_id"].(string); id != "" {
				operation.ID = id
				break
			}
		}
	}
	if operation.ID == "" {
		return application.ClaimOperation{}, fmt.Errorf("no operation id for claim %s of %s", bounded(fence, 12), instance)
	}
	current, err := e.made(ctx, "made_get_ceremony_instance", map[string]any{"ceremony_id": instance})
	if err != nil {
		// The operation id is already read: it is returned with the error,
		// so a report can still name the claim's operation.
		return operation, fmt.Errorf("attempt of claim %s of %s: %w", bounded(fence, 12), instance, err)
	}
	steps, _ := current["steps"].([]any)
	for _, raw := range steps {
		record, _ := raw.(map[string]any)
		if record["step_id"] != step {
			continue
		}
		// A repeated step may be listed once per attempt; the claim is on
		// the highest, whatever order the records come in.
		if attempt, _ := record["attempt"].(float64); attempt >= 1 && int(attempt) > operation.Attempt {
			operation.Attempt = int(attempt)
		}
	}
	if operation.Attempt == 0 {
		return operation, fmt.Errorf("no attempt for step %s of %s", step, instance)
	}
	return operation, nil
}

// Report sends one host status report. The host agent id is the claim's
// owner, as MADE requires; the install id and the run token travel as the
// incarnation.
func (e Engine) Report(ctx context.Context, report application.HostReport) error {
	status := map[string]any{
		"ceremony_id": report.Instance, "step_id": report.Step, "claim_fence": report.Fence, "operation_id": report.OperationID,
		"agent_execution_id": report.Execution, "logical_worker_id": report.Worker, "claim_owner_id": e.owner(),
		"host_agent_id": e.owner(), "host_agent_incarnation": report.Host.Incarnation, "role_id": report.Role,
		"attempt": report.Attempt, "execution_status": report.Status, "liveness": "fresh", "source": "host_report", "activity": report.Activity,
		"task_summary": bounded(report.Summary, 500), "observed_at": time.Now().UTC().Format(time.RFC3339),
		"report_sequence": report.Sequence, "idempotency_key": fmt.Sprintf("axlr:report:%s:%s:%s:%d", report.Instance, report.Step, bounded(report.Fence, 12), report.Sequence),
	}
	if report.Host.AgentID != "" {
		status["host_agent_incarnation"] = report.Host.AgentID + ":" + report.Host.Incarnation
	}
	if status["host_agent_incarnation"] == "" {
		status["host_agent_incarnation"] = "unknown"
	}
	if report.Model != "" {
		status["requested_model"], status["actual_model"] = report.Model, report.Model
	}
	_, err := e.made(ctx, "made_report_ceremony_agent_status", map[string]any{"status": status})
	return err
}

// Renew extends a live claim's lease by lease from now.
func (e Engine) Renew(ctx context.Context, instance, step, fence string, lease time.Duration) error {
	ttl := int64(claimLease)
	if lease > 0 {
		ttl = lease.Milliseconds()
	}
	_, err := e.made(ctx, "made_renew_ceremony_step_lease", map[string]any{"ceremony_id": instance, "step_id": step, "claim_fence": fence, "lease_owner_id": e.owner(), "renewal_id": fmt.Sprintf("axlr:renew:%s:%s:%d", instance, step, time.Now().UnixNano()), "lease_ttl_ms": ttl})
	return err
}

// Handoff journals a lost host incarnation for a live claim.
func (e Engine) Handoff(ctx context.Context, handoff application.HostHandoff) error {
	declaration := map[string]any{"id": fmt.Sprintf("axlr:handoff:%s:%s", handoff.Instance, bounded(handoff.Fence, 16)), "step_id": handoff.Step, "claim_fence": handoff.Fence, "owner": e.owner(), "incarnation": handoff.Incarnation, "state": "lost", "observed_at": time.Now().UTC().Format(time.RFC3339), "evidence": bounded(handoff.Evidence, 2000)}
	_, err := e.made(ctx, "made_record_ceremony_host_handoff", map[string]any{"ceremony_id": handoff.Instance, "declaration": declaration})
	return err
}
