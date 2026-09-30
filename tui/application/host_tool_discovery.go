package application

import (
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
	"strings"
)

func hostDiscover(snapshot []domain.AvailableTool, arguments root.JSONValue) (any, error) {
	args, err := decodeHostArguments(arguments, "query", "name", "limit", "offset")
	if err != nil {
		return nil, err
	}
	if _, _, err := hostFindTool(snapshot, ""); err != nil {
		return nil, err
	}
	limit, err := hostInteger(args, "limit", 8)
	if err != nil || limit < 1 || limit > 20 {
		return nil, errors.New("limit must be an integer between 1 and 20")
	}
	offset, err := hostInteger(args, "offset", 0)
	if err != nil || offset < 0 {
		return nil, errors.New("offset must be a nonnegative integer")
	}
	if raw, exists := args["name"]; exists {
		if _, exists := args["offset"]; exists {
			return nil, errors.New("offset is only supported for search")
		}
		if _, exists := args["query"]; exists {
			return nil, errors.New("use name for exact schema or query for search, not both")
		}
		var name string
		if err := json.Unmarshal(raw, &name); err != nil || name == "" {
			return nil, errors.New("name must be a nonempty exact tool name")
		}
		tool, known, err := hostFindTool(snapshot, root.ToolName(name))
		if err != nil {
			return nil, err
		}
		if !known || tool.Identity.Kind != domain.ToolKindPlugin {
			return nil, fmt.Errorf("unknown registered plugin tool %q", name)
		}
		if _, err := root.NewJSONObject(tool.Definition.Parameters.Bytes()); err != nil {
			return nil, errors.New("registered tool has no valid object schema")
		}
		return map[string]any{"name": tool.Definition.Name, "plugin": tool.Identity.Plugin.PluginID, "tool": tool.Identity.Plugin.ToolName, "description": tool.Definition.Description, "parameters": tool.Definition.Parameters}, nil
	}
	query := ""
	if raw, exists := args["query"]; exists {
		var value *string
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return nil, errors.New("query must be a string")
		}
		query = *value
		if len(query) > 512 {
			return nil, errors.New("query exceeds 512 bytes")
		}
	}
	terms := strings.Fields(strings.ToLower(query))
	plugins := make([]domain.AvailableTool, 0, len(snapshot))
	for _, tool := range snapshot {
		if tool.Identity.Kind == domain.ToolKindPlugin {
			plugins = append(plugins, tool)
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Definition.Name < plugins[j].Definition.Name })
	summaries := make([]map[string]any, 0, limit)
	matched := 0
	for _, tool := range plugins {
		text := strings.ToLower(string(tool.Definition.Name) + " " + tool.Identity.Plugin.PluginID.String() + " " + tool.Identity.Plugin.ToolName.String() + " " + string(tool.Definition.Description))
		matches := true
		for _, term := range terms {
			if !strings.Contains(text, term) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		matched++
		if matched > offset && len(summaries) < limit {
			description := []rune(strings.Join(strings.Fields(string(tool.Definition.Description)), " "))
			if len(description) > 160 {
				description = append(description[:160], '…')
			}
			summaries = append(summaries, map[string]any{"name": tool.Definition.Name, "plugin": tool.Identity.Plugin.PluginID, "tool": tool.Identity.Plugin.ToolName, "description": string(description)})
		}
	}
	if offset > matched {
		return nil, errors.New("offset exceeds matching tools")
	}
	next := offset + len(summaries)
	return map[string]any{"tools": summaries, "total_matches": matched, "has_more": next < matched, "next_offset": next}, nil
}

func hostInteger(args map[string]json.RawMessage, key string, fallback int) (int, error) {
	raw, exists := args[key]
	if !exists {
		return fallback, nil
	}
	var value *int
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return *value, nil
}
