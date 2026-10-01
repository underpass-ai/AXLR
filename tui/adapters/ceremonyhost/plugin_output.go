package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// refusal is a tool error the plugin returned with a machine code.
type refusal struct{ Code, Message string }

func (r *refusal) Error() string { return r.Code + ": " + r.Message }

// callPlugin runs one plugin tool through AXLR's executor, as the work
// identity, and decodes its JSON result. Host-initiated calls do not pass
// through the model's approval policy.
func callPlugin(ctx context.Context, tools application.ToolExecutionPort, plugin, tool string, arguments map[string]any) (map[string]any, error) {
	if tools == nil {
		return nil, errors.New("tool executor is required")
	}
	id, err := domain.NewPluginToolIdentity(root.PluginRef{PluginID: root.PluginID(plugin), ToolName: root.PluginToolName(tool)})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	args, err := root.NewJSONObject(encoded)
	if err != nil {
		return nil, err
	}
	outcome, err := tools.Execute(ctx, id, args)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Status string `json:"status"`
		Output struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Structured      json.RawMessage `json:"structured_content"`
			StructuredCamel json.RawMessage `json:"structuredContent"`
			IsError         bool            `json:"is_error"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &envelope); err != nil {
		return nil, fmt.Errorf("%s: unreadable result", tool)
	}
	if envelope.Error != nil && envelope.Error.Message != "" && len(envelope.Output.Content) == 0 {
		return nil, fmt.Errorf("%s: %s", tool, envelope.Error.Message)
	}
	payload := map[string]any{}
	raw := envelope.Output.Structured
	if len(raw) == 0 {
		raw = envelope.Output.StructuredCamel
	}
	if len(raw) > 0 && string(raw) != "null" {
		err = json.Unmarshal(raw, &payload)
	} else if len(envelope.Output.Content) > 0 {
		if json.Unmarshal([]byte(envelope.Output.Content[0].Text), &payload) != nil {
			payload = map[string]any{"message": envelope.Output.Content[0].Text}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%s: unreadable result", tool)
	}
	if outcome.IsError || envelope.Output.IsError {
		code, _ := payload["code"].(string)
		message, _ := payload["message"].(string)
		if nested, ok := payload["error"].(map[string]any); ok {
			code, _ = nested["code"].(string)
			message, _ = nested["message"].(string)
		}
		if message == "" {
			message = "refused"
		}
		return nil, &refusal{Code: code, Message: message}
	}
	return payload, nil
}
