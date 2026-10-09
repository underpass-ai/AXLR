package axlr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ToolRunner delegates validation and effects to AXLR. It never retries a call.
type ToolRunner struct {
	Executor    *runtime.Executor
	Diagnostics application.DiagnosticPort
	// KMPGuideRoot is the plugin directory of the connected KMP engine,
	// whose guide assets a store without a guide is synced from.
	KMPGuideRoot string
	// LogFailure, when set, records a call that failed inside AXLR or its
	// plugin transport in the console's app log, whose reader axlr_logs the
	// failure then names.
	LogFailure func(string)
}

// logHint ends the message of a failure the app log recorded.
const logHint = " (the console log may say more: axlr_logs)"

func (r ToolRunner) Execute(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) (out domain.ToolOutcome, returnErr error) {
	ctx, span := application.StartDiagnosticSpan(ctx, r.Diagnostics, application.DiagnosticActionToolExecution, application.DiagnosticEvent{Bytes: len(args.Bytes())})
	responseClass := application.DiagnosticErrorNone
	defer func() {
		class := responseClass
		if class == application.DiagnosticErrorNone && (returnErr != nil || out.IsError || out.Uncertain) {
			class = application.DiagnosticErrorTool
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			class = application.DiagnosticErrorCancelled
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			class = application.DiagnosticErrorTimeout
		}
		span.End(class)
	}()
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
	change := r.prepareChange(ctx, id, args)
	response := r.Executor.Execute(ctx, request)
	if response.Status == "timed_out" {
		responseClass = application.DiagnosticErrorTimeout
	} else if response.Status == "cancelled" {
		responseClass = application.DiagnosticErrorCancelled
	}
	out = domain.ToolOutcome{IsError: response.Status != "completed", Uncertain: response.Status == "timed_out" || response.Status == "cancelled"}
	out.Change = completedChange(change, response)
	// The root DTO maps unclassified plugin transport/protocol errors to
	// failed/internal_error, including a lost reply after an effect. It carries
	// no execution-stage proof, so retain uncertainty for that case. Rejected
	// requests establish nonexecution; completed MCP is_error results are definite.
	if id.Kind == domain.ToolKindPlugin && response.Status == "failed" && response.Error != nil && response.Error.Code == "internal_error" {
		out.Uncertain = true
	}
	if r.LogFailure != nil && response.Error != nil && (response.Error.Code == "internal_error" || response.Status == "timed_out") {
		target := "local_" + id.LocalOperation
		if id.Kind == domain.ToolKindPlugin {
			target = "plugin " + id.Plugin.PluginID.String() + " tool " + id.Plugin.ToolName.String()
		}
		r.LogFailure(target + " " + response.Status + ": " + response.Error.Code + ": " + response.Error.Message)
		failure := *response.Error
		failure.Message += logHint
		response.Error = &failure
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
	out.Content, err = root.NewText(r.namedGuideRoot(id, string(content)))
	return out, err
}

// namedGuideRoot fills the plugin root into KMP's answer for a store
// without a guide: it names `kmp-mcp guide sync --plugin-root <plugin-root>`
// and the console knows which directory that is. The sync must run with
// the engine stopped, so the console cannot run it for the model.
func (r ToolRunner) namedGuideRoot(id domain.ToolIdentity, content string) string {
	if r.KMPGuideRoot == "" || id.Kind != domain.ToolKindPlugin || id.Plugin.PluginID != "kmp" {
		return content
	}
	escaped, err := json.Marshal(r.KMPGuideRoot)
	if err != nil {
		return content
	}
	// The answer is JSON, which writes < and > as \u003c and \u003e; the
	// path goes in as a JSON string without its quotes.
	path := string(escaped[1 : len(escaped)-1])
	content = strings.ReplaceAll(content, `\u003cplugin-root\u003e`, path)
	return strings.ReplaceAll(content, "<plugin-root>", path)
}
