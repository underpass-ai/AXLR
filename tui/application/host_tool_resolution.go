package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"io"
	"strings"
)

// ResolveToolCall resolves against the frozen authority snapshot. The wrapper
// has no execution privilege: callers must approve the returned real identity.
// The original call ID and history are unchanged.
func ResolveToolCall(snapshot []domain.AvailableTool, call root.ToolCall) (domain.AvailableTool, root.JSONValue, bool, error) {
	tool, known, err := hostFindTool(snapshot, call.Name)
	if err == nil {
		err = malformedArguments(call.Arguments)
	}
	if err != nil || !known {
		return domain.AvailableTool{}, root.JSONValue{}, known, err
	}
	if refusal, hidden := hiddenFromModel(tool); hidden {
		// Called by its own name, not through the bridge: refused all the
		// same (until 14 October 2026 such a call resolved and ran).
		return domain.AvailableTool{}, root.JSONValue{}, true, errors.New(refusal)
	}
	if tool.Identity.Kind != domain.ToolKindHost || tool.Identity.LocalOperation != domain.HostOperationCallTool {
		return tool, call.Arguments, true, nil
	}
	args, err := decodeHostArguments(call.Arguments, "name", "arguments", WhyArgument)
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
	if refusal, hidden := hiddenFromModel(target); hidden {
		return domain.AvailableTool{}, root.JSONValue{}, true, errors.New(refusal)
	}
	return target, arguments, true, nil
}

// hiddenFromModel reports a plugin tool the console keeps to itself and
// the refusal the model reads (decisions D6 and G6 of the lifecycle
// integration design): the bridge and a direct call by its name refuse it
// and axlr_tools neither lists nor describes it, so the model never sees
// its schema; the console's own calls do not pass here. The engine adapter decides it (the tool's
// surface).
func hiddenFromModel(tool domain.AvailableTool) (string, bool) {
	if tool.Identity.Kind != domain.ToolKindPlugin || tool.Surface.Hidden == "" {
		return "", false
	}
	return tool.Surface.Hidden, true
}

// malformedArgumentsKey names the object the model adapter
// (adapters/openrouter/response_mapper.go) puts in place of a call's
// arguments that were not a JSON object, such as a call the output limit cut
// short; keep the two names in step.
const malformedArgumentsKey = "axlr_malformed_arguments"

// malformedArguments refuses a call whose arguments the adapter could not
// read, with the reason, so every resolution path records a tool error the
// model can correct instead of running or pausing on it.
func malformedArguments(arguments root.JSONValue) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &fields) != nil {
		return nil
	}
	raw, found := fields[malformedArgumentsKey]
	if !found {
		return nil
	}
	var malformed struct {
		Error            string `json:"error"`
		CutByOutputLimit bool   `json:"cut_by_output_limit"`
	}
	_ = json.Unmarshal(raw, &malformed)
	if malformed.Error == "" {
		malformed.Error = "unknown error"
	}
	if malformed.CutByOutputLimit {
		return fmt.Errorf("the model's output limit cut the call's arguments (%s); send the complete call again, shorter", malformed.Error)
	}
	// The history keeps this stand-in as the call's arguments: on 16 October
	// 2026 claude-haiku-5.5 read it there as a field it had sent and spent
	// ten calls apologising for it.
	return fmt.Errorf("the call's arguments were not a valid JSON object (%s); %s is the console's stand-in for the unreadable arguments, not a field you sent: send the complete call again with the tool's own fields", malformed.Error, malformedArgumentsKey)
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
// Shape errors name the accepted fields so the model can correct the call
// instead of guessing another field name.
func decodeHostArguments(value root.JSONValue, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(value.Bytes()))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("host arguments must be an object; accepted: %s", strings.Join(allowed, ", "))
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
		if ok && key == WhyArgument && !fields[key] {
			// A why on a host tool that asks none is read and dropped, as
			// on a local read: the model gives them to everything once told.
			var ignored json.RawMessage
			if err := d.Decode(&ignored); err != nil {
				return nil, err
			}
			continue
		}
		if !ok || !fields[key] {
			return nil, fmt.Errorf("unknown host argument %q; accepted: %s", key, strings.Join(allowed, ", "))
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
