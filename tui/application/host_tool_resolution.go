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

// The plugin verbs the console keeps to itself (decisions D6 and G6 of the
// lifecycle integration design): the bridge refuses them and axlr_tools
// neither lists nor describes them, so the model never sees their schema;
// the console's own calls do not pass here.

// consoleOwnedRefusal answers a model that reaches for KMP's identity:
// registering an agent, opening a context and choosing the store are the
// console's alone, and it says what to do instead, since the guidance of
// older sessions still names kmp_guide.
const consoleOwnedRefusal = "kmp_guide is the console's: it registered this session with KMP and passes the session's context on every KMP call; ask memory with axlr_recall, or call kmp_ask, kmp_wake and kmp_write_memory through axlr_call_tool without context_id"

// ceremonyOwnedRefusal answers a model that reaches for MADE's write
// surface: the console starts, claims, completes and transitions every
// ceremony, this session's included.
const ceremonyOwnedRefusal = "the console runs ceremonies: it starts, claims, completes, transitions, publishes and cancels them itself, this session's included; read definitions and instances with the made_get_*, made_list_*, made_read_*, made_search_* and made_inspect_* verbs, design and validate drafts, and ask the person to run or publish one"

// madeReadVerbs are the MADE verbs the model may call: reads, searches,
// streams and design-time checks; nothing that starts, claims, completes,
// transitions, cancels, pauses, resumes, runs, approves, defers, grants,
// revokes, publishes, registers, deletes or uploads.
var (
	madeReadPrefixes = []string{"list_", "get_", "read_", "stream_", "search_", "inspect_"}
	madeReadVerbs    = map[string]bool{"get_help": true, "discover_capabilities": true, "design_ceremony": true, "validate_ceremony_draft": true, "explain_ceremony_draft": true, "diff_ceremony_definitions": true, "render_agentic_system_diagram": true, "generate_ceremony_report": true, "verify_ceremony_journal": true}
)

// hiddenFromModel reports a plugin tool the model may not call and why.
func hiddenFromModel(tool domain.AvailableTool) (string, bool) {
	if tool.Identity.Kind != domain.ToolKindPlugin {
		return "", false
	}
	name := string(tool.Identity.Plugin.ToolName)
	switch tool.Identity.Plugin.PluginID {
	case "kmp":
		if name == "kmp_guide" {
			return consoleOwnedRefusal, true
		}
	case "made":
		verb := strings.TrimPrefix(name, "made_")
		if madeReadVerbs[verb] {
			return "", false
		}
		for _, prefix := range madeReadPrefixes {
			if strings.HasPrefix(verb, prefix) {
				return "", false
			}
		}
		return ceremonyOwnedRefusal, true
	}
	return "", false
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
	return fmt.Errorf("the call's arguments were not a valid JSON object (%s); send the complete call again", malformed.Error)
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
