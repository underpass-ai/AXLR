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
type Engine struct{ Tools application.ToolExecutionPort }

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
	claim, err := e.made(ctx, "made_claim_ceremony_step", map[string]any{"ceremony_id": instance, "step_id": step, "actor_kind": actorKind, "lease_owner_id": actorID, "idempotency_key": key, "lease_ttl_ms": ttl})
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
			if step != "" && fence != "" && phase == "live" && owner == actorID {
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
