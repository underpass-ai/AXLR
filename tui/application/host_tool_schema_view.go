package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// schemaViewBytes leaves room for the name, description and wrapper fields
// around a schema inside one host result.
const schemaViewBytes = MaxHostResultBytes - 4096

// schemaMaps and schemaLists name the keywords whose values hold subschemas;
// only those positions are walked, so a property called "x-..." is kept.
var (
	schemaSingles = []string{"items", "additionalProperties", "not", "if", "then", "else", "contains", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "additionalItems"}
	schemaMaps    = []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"}
	schemaLists   = []string{"oneOf", "anyOf", "allOf", "prefixItems"}
)

// toolSchemaView returns what axlr_tools shows for one schema: the node at
// path, without x-* annotations, or an outline when it still does not fit.
func toolSchemaView(parameters []byte, path string) (map[string]any, error) {
	var schema any
	if err := json.Unmarshal(parameters, &schema); err != nil {
		return nil, errors.New("registered tool has no valid object schema")
	}
	node, err := schemaPointer(schema, path)
	if err != nil {
		return nil, err
	}
	view := map[string]any{}
	if path != "" {
		view["path"] = path
	}
	omitted := map[string]bool{}
	stripped := stripSchemaAnnotations(node, omitted)
	if len(omitted) > 0 {
		names := make([]string, 0, len(omitted))
		for name := range omitted {
			names = append(names, name)
		}
		sort.Strings(names)
		view["annotations_omitted"] = names
	}
	encoded, err := json.Marshal(stripped)
	if err != nil {
		return nil, err
	}
	if len(encoded) <= schemaViewBytes {
		view["parameters"] = stripped
		return view, nil
	}
	switch value := stripped.(type) {
	case map[string]any:
		view["outline"] = schemaOutline(value, path)
	case []any:
		lines := make([]any, len(value))
		for i, child := range value {
			lines[i] = schemaLine(child, path+"/"+strconv.Itoa(i))
		}
		view["outline"] = map[string]any{"items": lines}
	default:
		return nil, fmt.Errorf("schema value at %q exceeds %d bytes and has no parts to select", path, schemaViewBytes)
	}
	view["schema_bytes"] = len(encoded)
	view["hint"] = "The schema is too large to return whole. Repeat with path set to one of the outline paths to get that part exactly."
	return view, nil
}

func stripSchemaAnnotations(node any, omitted map[string]bool) any {
	object, ok := node.(map[string]any)
	if !ok {
		return node
	}
	result := make(map[string]any, len(object))
	for key, value := range object {
		if strings.HasPrefix(key, "x-") {
			omitted[key] = true
			continue
		}
		result[key] = value
	}
	for _, key := range schemaSingles {
		if value, exists := result[key]; exists {
			result[key] = stripSchemaAnnotations(value, omitted)
		}
	}
	for _, key := range schemaMaps {
		if children, ok := result[key].(map[string]any); ok {
			copied := make(map[string]any, len(children))
			for name, child := range children {
				copied[name] = stripSchemaAnnotations(child, omitted)
			}
			result[key] = copied
		}
	}
	for _, key := range schemaLists {
		if children, ok := result[key].([]any); ok {
			copied := make([]any, len(children))
			for i, child := range children {
				copied[i] = stripSchemaAnnotations(child, omitted)
			}
			result[key] = copied
		}
	}
	return result
}

// schemaOutline keeps the node's own keywords and one line per property and
// alternative, enough to choose the next path.
func schemaOutline(object map[string]any, base string) map[string]any {
	outline := map[string]any{}
	for _, key := range []string{"type", "required", "additionalProperties", "description", "enum", "const"} {
		if value, exists := object[key]; exists {
			if text, isText := value.(string); isText {
				value = shortText(text)
			}
			if _, isObject := value.(map[string]any); isObject {
				value = "schema"
			}
			outline[key] = value
		}
	}
	for _, key := range schemaMaps {
		if children, ok := object[key].(map[string]any); ok {
			lines := make(map[string]any, len(children))
			for name, child := range children {
				lines[name] = schemaLine(child, base+"/"+key+"/"+escapePointer(name))
			}
			outline[key] = lines
		}
	}
	for _, key := range schemaLists {
		if children, ok := object[key].([]any); ok {
			lines := make([]any, len(children))
			for i, child := range children {
				lines[i] = schemaLine(child, base+"/"+key+"/"+strconv.Itoa(i))
			}
			outline[key] = lines
		}
	}
	if items, exists := object["items"]; exists {
		outline["items"] = schemaLine(items, base+"/items")
	}
	return outline
}

func schemaLine(node any, path string) map[string]any {
	line := map[string]any{"path": path}
	object, ok := node.(map[string]any)
	if !ok {
		return line
	}
	for _, key := range []string{"type", "$ref"} {
		if value, exists := object[key]; exists {
			line[key] = value
		}
	}
	if text, ok := object["description"].(string); ok {
		line["description"] = shortText(text)
	}
	if encoded, err := json.Marshal(node); err == nil {
		line["bytes"] = len(encoded)
	}
	return line
}

func shortText(text string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return string(runes)
}

func escapePointer(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
}

// schemaPointer resolves an RFC 6901 JSON pointer; "" is the whole schema.
func schemaPointer(schema any, path string) (any, error) {
	if path == "" {
		return schema, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("path must be a JSON pointer starting with /")
	}
	node := schema
	for _, raw := range strings.Split(path[1:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch value := node.(type) {
		case map[string]any:
			child, exists := value[token]
			if !exists {
				return nil, errors.New("path does not exist in the schema")
			}
			node = child
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(value) {
				return nil, errors.New("path does not exist in the schema")
			}
			node = value[index]
		default:
			return nil, errors.New("path does not exist in the schema")
		}
	}
	return node, nil
}
