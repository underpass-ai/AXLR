package axlr

import (
	"context"
	"encoding/json"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ToolRunner delegates validation and effects to AXLR. It never retries a call.
type ToolRunner struct{ Executor *runtime.Executor }

func (r ToolRunner) Execute(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	if err := id.Validate(); err != nil {
		return domain.ToolOutcome{}, err
	}
	if r.Executor == nil {
		return domain.ToolOutcome{}, errors.New("tool executor is required")
	}
	if _, err := root.NewJSONObject(args.Bytes()); err != nil {
		return domain.ToolOutcome{}, err
	}
	request := dto.Request{ProtocolVersion: runtime.ProtocolVersion, RequestID: "tui-tool", Tool: id.LocalOperation, Arguments: args.Bytes()}
	if id.Kind == domain.ToolKindPlugin {
		request.Tool = "plugins.call"
		encoded, err := json.Marshal(dto.PluginCallArgs{PluginID: id.Plugin.PluginID.String(), ToolName: id.Plugin.ToolName.String(), Arguments: args.Bytes()})
		if err != nil {
			return domain.ToolOutcome{}, err
		}
		request.Arguments = encoded
	}
	response := r.Executor.Execute(ctx, request)
	out := domain.ToolOutcome{IsError: response.Status != "completed", Uncertain: response.Status == "timed_out" || response.Status == "cancelled"}
	// The root DTO maps unclassified plugin transport/protocol errors to
	// failed/internal_error, including a lost reply after an effect. It carries
	// no execution-stage proof, so retain uncertainty for that case. Rejected
	// requests establish nonexecution; completed MCP is_error results are definite.
	if id.Kind == domain.ToolKindPlugin && response.Status == "failed" && response.Error != nil && response.Error.Code == "internal_error" {
		out.Uncertain = true
	}
	if plugin, ok := response.Output.(dto.PluginCallOutput); ok {
		out.IsError = out.IsError || plugin.IsError
	}
	if process, ok := response.Output.(dto.ExecOutput); ok {
		out.IsError = out.IsError || process.ExitCode != 0
	}
	content, err := json.Marshal(response)
	if err != nil {
		return domain.ToolOutcome{}, err
	}
	out.Content, err = root.NewText(string(content))
	return out, err
}
