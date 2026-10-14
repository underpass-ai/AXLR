package application

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// WhyArgument is the one-sentence reason the model gives each side effect
// (decision E2 of the lifecycle integration design): a file written or
// edited, a command, an external call through the bridge, a forged tool
// run. The console shows it on the approval card and keeps it in the turn's
// record; a call without it is refused, so the model supplies it in its
// next request. Reads, searches, listings and the console's own host tools
// need none.
const WhyArgument = "why"

// WhyMaxRunes bounds the reason: one sentence.
const WhyMaxRunes = 240

// MissingWhy is the refusal of a side-effect call without a why.
const MissingWhy = "give why: one sentence saying what this call is for"

// whyDescription is the property's description in the patched schemas.
const whyDescription = "One sentence saying what this call is for (required): the console shows it on the approval card and records it with the turn."

// whyTools are the tools whose calls carry a why.
var whyTools = map[root.ToolName]bool{"local_write": true, "local_edit": true, "local_exec": true, HostCallToolName: true, HostRunToolName: true}

// whyTolerated are the local reads, which need no why but accept one: on
// 13 October 2026 claude-haiku-5.5, told to give its writes and commands
// a why, gave its reads one too and the read schema refused the call.
var whyTolerated = map[root.ToolName]bool{"local_read": true, "local_search": true, "local_list": true}

// RequireWhy returns the snapshot with why required on every side-effect
// tool's schema and accepted on the local reads. The console applies it to
// the catalog it gives the model; the service does not, so its API clients
// keep the plain schemas.
func RequireWhy(tools []domain.AvailableTool) []domain.AvailableTool {
	out := make([]domain.AvailableTool, 0, len(tools))
	for _, tool := range tools {
		if whyTools[tool.Definition.Name] || whyTolerated[tool.Definition.Name] {
			if patched, err := withWhy(tool.Definition.Parameters, whyTools[tool.Definition.Name]); err == nil {
				tool.Definition.Parameters = patched
			}
		}
		out = append(out, tool)
	}
	return out
}

// withWhy adds the why property to a schema, and its requirement when
// required.
func withWhy(schema root.JSONValue, required bool) (root.JSONValue, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(schema.Bytes(), &object); err != nil {
		return schema, err
	}
	var properties map[string]json.RawMessage
	if raw, ok := object["properties"]; ok {
		if err := json.Unmarshal(raw, &properties); err != nil {
			return schema, err
		}
	}
	if properties == nil {
		properties = map[string]json.RawMessage{}
	}
	if _, present := properties[WhyArgument]; !present {
		description := whyDescription
		if !required {
			description = "Optional here: reads need no why."
		}
		property, _ := json.Marshal(map[string]any{"type": "string", "minLength": 1, "maxLength": WhyMaxRunes, "description": description})
		properties[WhyArgument] = property
	}
	encodedProperties, err := json.Marshal(properties)
	if err != nil {
		return schema, err
	}
	object["properties"] = encodedProperties
	if required {
		var names []string
		if raw, ok := object["required"]; ok {
			if err := json.Unmarshal(raw, &names); err != nil {
				return schema, err
			}
		}
		found := false
		for _, name := range names {
			found = found || name == WhyArgument
		}
		if !found {
			names = append(names, WhyArgument)
		}
		encodedRequired, err := json.Marshal(names)
		if err != nil {
			return schema, err
		}
		object["required"] = encodedRequired
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return schema, err
	}
	return root.NewJSONObject(encoded)
}

// RequiresWhy reports whether a tool's schema, as the snapshot froze it,
// requires a why: the console's catalog does, the service's does not.
func RequiresWhy(tool domain.AvailableTool) bool {
	if !whyTools[tool.Definition.Name] {
		return false
	}
	var object struct {
		Required []string `json:"required"`
	}
	if json.Unmarshal(tool.Definition.Parameters.Bytes(), &object) != nil {
		return false
	}
	for _, name := range object.Required {
		if name == WhyArgument {
			return true
		}
	}
	return false
}

// CallWhy reads a call's why; empty when absent.
func CallWhy(arguments root.JSONValue) string {
	var fields struct {
		Why string `json:"why"`
	}
	if json.Unmarshal(arguments.Bytes(), &fields) != nil {
		return ""
	}
	return strings.TrimSpace(fields.Why)
}

// whyError refuses a side-effect call whose snapshot requires a why and
// whose arguments lack one or overrun it. The message is the model's: it
// says what to send, not which schema rule failed.
func whyError(tool domain.AvailableTool, arguments root.JSONValue) error {
	if !RequiresWhy(tool) {
		return nil
	}
	why := CallWhy(arguments)
	if why == "" {
		return errors.New(string(tool.Definition.Name) + ": " + MissingWhy)
	}
	if utf8.RuneCountInString(why) > WhyMaxRunes {
		return errors.New(string(tool.Definition.Name) + ": why is one sentence, at most 240 characters")
	}
	return nil
}

// StripWhy returns the arguments without why: the runtime decodes a local
// tool's arguments strictly and the plugins know nothing of it.
func StripWhy(arguments root.JSONValue) root.JSONValue {
	var fields map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &fields) != nil {
		return arguments
	}
	if _, present := fields[WhyArgument]; !present {
		return arguments
	}
	delete(fields, WhyArgument)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return arguments
	}
	stripped, err := root.NewJSONObject(encoded)
	if err != nil {
		return arguments
	}
	return stripped
}
