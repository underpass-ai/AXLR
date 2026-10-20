package application

import (
	"encoding/json"
	"errors"
	"fmt"
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
const whyDescription = "One sentence in the person's language saying what this call is for (required): the console shows it on the approval card and records it with the turn."

// consoleLanguages names the console's languages for the model.
var consoleLanguages = map[string]string{"en": "English", "es": "Spanish"}

// languageGuidance tells the model the console's language, in which it
// writes each why: the person reads it on the approval card, among the
// console's own words. On 14 October 2026 glm-5.3-flash wrote every why in
// English in a Spanish console, the card reading "por qué: run the tests".
// The line is the same for every request of a console, so the prompt's
// cached prefix keeps it; an unknown or empty language adds none.
func languageGuidance(language string) string {
	name, ok := consoleLanguages[strings.ToLower(language)]
	if !ok {
		return ""
	}
	return "The person's console is in " + name + ": write each call's why in " + name + ".\n"
}

// whyTools are the tools whose calls carry a why: every side effect,
// including the host tools that write, replace or remove a workspace tool.
// On 20 October 2026 claude-haiku-5.5 sent axlr_forge_tool a why eight
// times, each refused as "arguments must match its schema" without naming
// the field, and the tool was never created: a forge is a write, so it
// takes the why every write takes.
var whyTools = map[root.ToolName]bool{"local_write": true, "local_edit": true, "local_exec": true, HostCallToolName: true, HostRunToolName: true, HostForgeToolName: true, HostRemoveToolName: true, HostInstallToolName: true}

// whyTolerated are the local reads, which need no why but accept one: on
// 13 October 2026 claude-haiku-5.5, told to give its writes and commands
// a why, gave its reads one too and the read schema refused the call.
var whyTolerated = map[root.ToolName]bool{"local_read": true, "local_search": true, "local_list": true}

// schemaError is a host tool's refusal of arguments that do not decode,
// naming what the decoder found (an unknown field, a wrong type), so the
// model can correct the call instead of resending it.
func schemaError(refusal string, err error) error {
	return fmt.Errorf("%s (%s)", refusal, strings.TrimPrefix(err.Error(), "json: "))
}

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

// requiredWhy is a host tool's own schema with why required. The console
// adds axlr_forge_tool, axlr_remove_tool and axlr_install_tool to a session
// after the catalog's RequireWhy, so their constructors carry it, as
// axlr_run_tool's literal schema does: on 20 October 2026 a forge without
// a why reached the card although whyTools named it.
func requiredWhy(schema root.JSONValue) root.JSONValue {
	if patched, err := withWhy(schema, true); err == nil {
		return patched
	}
	return schema
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
