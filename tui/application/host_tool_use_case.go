package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostToolUseCase provides read-only discovery and session recovery. Invocation
// wrappers are resolved separately and always use the target plugin's policy.
type HostToolUseCase struct{ Skills PluginSkillPort }

func (u HostToolUseCase) Execute(ctx context.Context, session domain.Session, identity domain.ToolIdentity, arguments root.JSONValue) (domain.ToolOutcome, error) {
	if err := ctx.Err(); err != nil {
		return domain.ToolOutcome{}, err
	}
	if err := identity.Validate(); err != nil {
		return hostFailure(err), nil
	}
	if identity.Kind != domain.ToolKindHost {
		return hostFailure(errors.New("not a host tool")), nil
	}
	var result any
	var err error
	switch identity.LocalOperation {
	case domain.HostOperationTools:
		result, err = hostDiscover(session.ToolSnapshot(), arguments)
	case domain.HostOperationHistory:
		result, err = hostHistory(session.Messages(), arguments)
	case domain.HostOperationSkill:
		result, err = u.readSkill(ctx, arguments)
	default:
		err = errors.New("invocation bridge must resolve and approve its exact plugin target")
	}
	if err != nil {
		return hostFailure(err), nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return hostFailure(err), nil
	}
	if len(encoded) > MaxHostResultBytes {
		return hostFailure(fmt.Errorf("host result exceeds %d bytes; use a narrower query or smaller page", MaxHostResultBytes)), nil
	}
	return domain.ToolOutcome{Content: root.Text(encoded)}, nil
}

func hostFailure(err error) domain.ToolOutcome {
	text := err.Error()
	// Snapshot aliases and hostile argument names can be large. Error outcomes
	// are bounded as well as successful host results.
	if len(text) > 1024 {
		text = text[:1024]
	}
	encoded, _ := json.Marshal(map[string]string{"error": text})
	return domain.ToolOutcome{Content: root.Text(encoded), IsError: true}
}
