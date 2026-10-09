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

func hostDiscover(snapshot []domain.AvailableTool, forged []ForgedTool, arguments root.JSONValue) (any, error) {
	args, err := decodeHostArguments(arguments, "query", "name", "path", "limit", "offset")
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
	path := ""
	if raw, exists := args["path"]; exists {
		if _, named := args["name"]; !named {
			return nil, errors.New("path requires name")
		}
		if err := json.Unmarshal(raw, &path); err != nil || len(path) > 512 {
			return nil, errors.New("path must be a JSON pointer string of at most 512 bytes")
		}
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
			if made, found := findForged(forged, name); found {
				return forgedView(made, path)
			}
			return nil, fmt.Errorf("unknown registered plugin tool %q", name)
		}
		if _, err := root.NewJSONObject(tool.Definition.Parameters.Bytes()); err != nil {
			return nil, errors.New("registered tool has no valid object schema")
		}
		view, err := toolSchemaView(tool.Definition.Parameters.Bytes(), path)
		if err != nil {
			return nil, err
		}
		view["name"], view["plugin"], view["tool"], view["description"] = tool.Definition.Name, tool.Identity.Plugin.PluginID, tool.Identity.Plugin.ToolName, tool.Definition.Description
		return view, nil
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
	// Forged tools follow the plugin tools under the same search; they are
	// called through axlr_run_tool.
	candidates := make([]discoveryCandidate, 0, len(plugins)+len(forged))
	for _, tool := range plugins {
		candidates = append(candidates, discoveryCandidate{
			text:    string(tool.Definition.Name) + " " + tool.Identity.Plugin.PluginID.String() + " " + tool.Identity.Plugin.ToolName.String() + " " + string(tool.Definition.Description),
			summary: map[string]any{"name": tool.Definition.Name, "plugin": tool.Identity.Plugin.PluginID, "tool": tool.Identity.Plugin.ToolName},
			about:   string(tool.Definition.Description),
		})
	}
	sorted := append([]ForgedTool(nil), forged...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, tool := range sorted {
		candidates = append(candidates, discoveryCandidate{
			text:    tool.Name + " forged " + tool.Description,
			summary: map[string]any{"name": tool.Name, "forged": true, "call_with": HostRunToolName},
			about:   tool.Description,
		})
	}
	// Every term must match. When none does, a tool matching some terms is
	// still better than nothing: those come back, most terms first, with
	// partial set (seen on 9 Oct 2026: "ventas total facturado producto csv"
	// found no tool described as summing units by price per product).
	selected := make([]discoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if termsMatched(candidate.text, terms) == len(terms) {
			selected = append(selected, candidate)
		}
	}
	partial := false
	if len(selected) == 0 && len(terms) > 1 {
		type ranked struct {
			candidate discoveryCandidate
			score     int
		}
		var some []ranked
		for _, candidate := range candidates {
			if score := termsMatched(candidate.text, terms); score > 0 {
				some = append(some, ranked{candidate, score})
			}
		}
		sort.SliceStable(some, func(i, j int) bool { return some[i].score > some[j].score })
		for _, item := range some {
			selected = append(selected, item.candidate)
		}
		partial = len(selected) > 0
	}
	matched := len(selected)
	summaries := make([]map[string]any, 0, limit)
	for i, candidate := range selected {
		if i >= offset && len(summaries) < limit {
			description := []rune(strings.Join(strings.Fields(candidate.about), " "))
			if len(description) > 160 {
				description = append(description[:160], '…')
			}
			candidate.summary["description"] = string(description)
			summaries = append(summaries, candidate.summary)
		}
	}
	if offset > matched {
		return nil, errors.New("offset exceeds matching tools")
	}
	next := offset + len(summaries)
	result := map[string]any{"tools": summaries, "total_matches": matched, "has_more": next < matched, "next_offset": next}
	if partial {
		result["partial"] = true
	}
	return result, nil
}

// termsMatched counts the lowercase terms text contains.
func termsMatched(text string, terms []string) int {
	text = strings.ToLower(text)
	count := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			count++
		}
	}
	return count
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

// discoveryCandidate is one plugin or forged tool under an axlr_tools search.
type discoveryCandidate struct {
	text, about string
	summary     map[string]any
}

// forgedView is axlr_tools' answer for one forged tool's exact name: its
// schema, or the part path selects, and how it runs.
func forgedView(tool ForgedTool, path string) (any, error) {
	if _, err := root.NewJSONObject(tool.InputSchema); err != nil {
		return nil, fmt.Errorf("forged tool %q has no valid input_schema; forge it again", tool.Name)
	}
	view, err := toolSchemaView(tool.InputSchema, path)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(tool.Files))
	for file := range tool.Files {
		files = append(files, ForgedToolsDir+"/"+tool.Name+"/"+file)
	}
	sort.Strings(files)
	view["name"], view["forged"], view["call_with"], view["description"] = tool.Name, true, HostRunToolName, tool.Description
	view["command"], view["files"] = append([]string{tool.Program}, tool.Args...), files
	return view, nil
}
