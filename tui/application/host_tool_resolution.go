package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"io"
)

// ResolveToolCall resolves against the frozen authority snapshot. The wrapper
// has no execution privilege: callers must approve the returned real identity.
// The original call ID and history are unchanged.
func ResolveToolCall(snapshot []domain.AvailableTool, call root.ToolCall) (domain.AvailableTool, root.JSONValue, bool, error) {
	tool, known, err := hostFindTool(snapshot, call.Name)
	if err != nil || !known {
		return domain.AvailableTool{}, root.JSONValue{}, known, err
	}
	if tool.Identity.Kind != domain.ToolKindHost || tool.Identity.LocalOperation != domain.HostOperationCallTool {
		return tool, call.Arguments, true, nil
	}
	args, err := decodeHostArguments(call.Arguments, "name", "arguments")
	if err != nil {
		return domain.AvailableTool{}, root.JSONValue{}, true, err
	}
	var name string
	if err := json.Unmarshal(args["name"], &name); err != nil || name == "" {
		return domain.AvailableTool{}, root.JSONValue{}, true, errors.New("axlr_call_tool requires a nonempty exact tool name")
	}
	arguments, err := root.NewJSONObject(args["arguments"])
	if err != nil {
		return domain.AvailableTool{}, root.JSONValue{}, true, errors.New("axlr_call_tool arguments must be an object")
	}
	target, found, err := hostFindTool(snapshot, root.ToolName(name))
	if err != nil {
		return domain.AvailableTool{}, root.JSONValue{}, true, err
	}
	if !found {
		return domain.AvailableTool{}, root.JSONValue{}, true, fmt.Errorf("unknown registered plugin tool %q", name)
	}
	if target.Identity.Kind != domain.ToolKindPlugin {
		return domain.AvailableTool{}, root.JSONValue{}, true, errors.New("axlr_call_tool can only call registered plugin tools")
	}
	return target, arguments, true, nil
}

func hostFindTool(snapshot []domain.AvailableTool, name root.ToolName) (domain.AvailableTool, bool, error) {
	var found domain.AvailableTool
	known := false
	seen := make(map[root.ToolName]bool, len(snapshot))
	for _, tool := range snapshot {
		if seen[tool.Definition.Name] {
			return domain.AvailableTool{}, false, fmt.Errorf("ambiguous tool alias %q", tool.Definition.Name)
		}
		seen[tool.Definition.Name] = true
		if err := tool.Identity.Validate(); err != nil {
			return domain.AvailableTool{}, false, err
		}
		if tool.Definition.Name == name {
			found, known = tool, true
		}
	}
	return found, known, nil
}

// Strict decoding rejects duplicate or unknown bridge fields before authority
// resolution, so malformed wrapper objects cannot change which tool is approved.
func decodeHostArguments(value root.JSONValue, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(value.Bytes()))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("host arguments must be an object")
	}
	fields := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		fields[key] = true
	}
	result := make(map[string]json.RawMessage)
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !fields[key] {
			return nil, fmt.Errorf("unknown host argument %q", key)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate host argument %q", key)
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, err
		}
		result[key] = raw
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("extra host argument data")
	}
	return result, nil
}
