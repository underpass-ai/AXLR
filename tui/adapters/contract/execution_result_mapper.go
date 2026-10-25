package contract

import (
	"encoding/json"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// ExecutionResult is the wire form of a result. Its arrays are never null,
// so a reader indexes them without a check.
func ExecutionResult(result domain.ExecutionResult) dto.ExecutionResult {
	out := dto.ExecutionResult{
		Schema:    dto.ExecutionResultSchema,
		Status:    string(result.Status),
		Session:   string(result.Session),
		Sessions:  make([]string, 0, len(result.Sessions)),
		Answer:    string(result.Answer),
		Error:     result.Error,
		Calls:     make([]dto.ExecutionCall, 0, len(result.Calls)),
		Artifacts: make([]dto.ExecutionArtifact, 0, len(result.Artifacts)),
		Usage:     executionUsage(result.Usage),
	}
	for _, id := range result.Sessions {
		out.Sessions = append(out.Sessions, string(id))
	}
	for _, call := range result.Calls {
		out.Calls = append(out.Calls, dto.ExecutionCall{ID: string(call.ID), Tool: string(call.Tool), Decision: string(call.Decision), Status: string(call.Status)})
	}
	for _, artifact := range result.Artifacts {
		out.Artifacts = append(out.Artifacts, ExecutionArtifact(artifact))
	}
	if result.Pending != nil {
		out.Pending = &dto.ExecutionCall{ID: string(result.Pending.ID), Tool: string(result.Pending.Name), Arguments: json.RawMessage(result.Pending.Arguments.Bytes())}
	}
	if result.Correlation != nil {
		correlation := ExecutionCorrelation(*result.Correlation)
		out.Correlation = &correlation
	}
	return out
}
