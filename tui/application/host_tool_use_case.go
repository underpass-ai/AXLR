package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// HostToolUseCase provides discovery, session recovery and session bookkeeping. Invocation
// wrappers are resolved separately and always use the target plugin's policy.
type HostToolUseCase struct {
	Skills PluginSkillPort
	Labels SessionLabelsPort
	// Repairs serves axlr_request_repair and axlr_repair_status; nil means
	// the console does not offer self-repair.
	Repairs RepairRequestPort
	// Judge serves axlr_judge; nil means Jev is not enabled.
	Judge JudgementPort
	// Windows bounds an axlr_history page to the projection's tool result
	// budget for the session's model; nil keeps MaxHistoryReadBytes.
	Windows ModelContextWindowPort
}

// historyPageLimit is the largest encoded axlr_history page that the
// projection of the session's model keeps whole: its budget, or the compact
// one when a compact ceremony step is running, as the continuation sizes it.
func (u HostToolUseCase) historyPageLimit(session domain.Session) int {
	if u.Windows == nil {
		return MaxHistoryReadBytes
	}
	budget := u.Windows.ContextBudget(sessionModel(session))
	if _, _, compact := compactRun(session); compact {
		budget = budget.Smaller(domain.CompactContextBudget())
	}
	return min(MaxHistoryReadBytes, budget.ToolResultBytes())
}

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
		result, err = hostHistory(session.Messages(), arguments, u.historyPageLimit(session))
	case domain.HostOperationSkill:
		result, err = u.readSkill(ctx, arguments)
	case domain.HostOperationSession:
		result, err = u.sessionContext(ctx, session, arguments)
	case domain.HostOperationStepDone:
		err = errors.New("ceremony steps are handled by the ceremony driver")
	case domain.HostOperationRequestRepair, domain.HostOperationRepairStatus, domain.HostOperationRequestImprovement:
		if u.Repairs == nil {
			err = errors.New("self-repair and self-improvement are not available in this console: they need MADE prepared and a repair repository configured")
			break
		}
		switch identity.LocalOperation {
		case domain.HostOperationRequestRepair:
			result, err = u.Repairs.Request(ctx, session, arguments)
		case domain.HostOperationRequestImprovement:
			result, err = u.Repairs.RequestImprovement(ctx, session, arguments)
		default:
			result, err = u.Repairs.Status(ctx, session, arguments)
		}
	case domain.HostOperationJudge:
		if u.Judge == nil {
			err = errors.New("Jev is not enabled in this console")
			break
		}
		result, err = hostJudge(ctx, u.Judge, arguments)
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
