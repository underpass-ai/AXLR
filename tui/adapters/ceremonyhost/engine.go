package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
			return application.ErrCeremonyNotPrepared
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

func (e Engine) Claim(ctx context.Context, instance, step, key string) (string, error) {
	claim, err := e.made(ctx, "made_claim_ceremony_step", map[string]any{"ceremony_id": instance, "step_id": step, "actor_kind": actorKind, "lease_owner_id": actorID, "idempotency_key": key, "lease_ttl_ms": claimLease})
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
	claimable, _ := current["claimable_step_ids"].([]any)
	for _, raw := range claimable {
		if step, _ := raw.(string); step != "" {
			view.Claimable = append(view.Claimable, step)
		}
	}
	return view, nil
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
