package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// withRememberAbouts gives axlr_remember's about, in a session whose
// memory the execution's policy bounds, the abouts it may write: an enum,
// the default first, and a description that says to leave it out. On 12
// November 2026 Codex models (codex/gpt-6.1-terra-high), which fill every
// field of a schema, invented an about that meant "none"
// (project:dev.void, project:dev.void2…, project:dev.ready-unused): each
// was refused, and one bot sent it ten times. The enum goes in the request
// only, not in the session's snapshot: the snapshot's schema checks the
// call, so a value outside it still reaches checkRememberAbout, whose
// refusal says which about to use.
func withRememberAbouts(tools []root.ToolDefinition, s domain.Session) []root.ToolDefinition {
	if !memoryBound(s) || !hasTool(tools, HostRememberName) {
		return tools
	}
	parameters, err := rememberParameters(boundAbouts(s.Export()))
	if err != nil {
		return tools
	}
	out := make([]root.ToolDefinition, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Name == HostRememberName {
			out[i].Parameters = parameters
		}
	}
	return out
}

// rememberParameters is axlr_remember's schema with about bounded to
// abouts, the first being where a write goes when about is left out.
func rememberParameters(abouts []string) (root.JSONValue, error) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(rememberSchema), &schema); err != nil {
		return root.JSONValue{}, err
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(abouts) == 0 {
		return root.JSONValue{}, errors.New("axlr_remember's schema has no properties to bound")
	}
	properties["about"] = map[string]any{
		"type": "string",
		"enum": abouts,
		"description": fmt.Sprintf("Leave it out: the memory goes under %s. Name one only to write elsewhere, and then exactly one of %s; no other about is accepted, and no value means empty or unused.",
			abouts[0], strings.Join(abouts, ", ")),
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return root.JSONValue{}, err
	}
	return root.NewJSONObject(data)
}
