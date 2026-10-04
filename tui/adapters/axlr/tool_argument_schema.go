package axlr

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// normalizeToolSchema restores exact enum/const numbers that Schema's JSON
// unmarshaller otherwise rounds to float64. Numeric bounds must be exactly
// representable by this library; unsupported precision is rejected explicitly.
func normalizeToolSchema(schema *jsonschema.Schema, value any, bounds map[float64]bool) error {
	if _, ok := value.(bool); ok {
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("schema nodes must be objects or booleans")
	}
	for key := range schema.Extra {
		// x-* keywords are vendor annotations (MADE's pattern catalogue and
		// x-made-shape explain the schema), never argument assertions. Other
		// custom keywords are unsupported.
		if !strings.HasPrefix(key, "x-") {
			return fmt.Errorf("unsupported schema keyword %q", key)
		}
	}
	if schema.Schema != "" && schema.Schema != "https://json-schema.org/draft/2020-12/schema" && schema.Schema != "http://json-schema.org/draft-07/schema#" && schema.Schema != "https://json-schema.org/draft-07/schema#" {
		return errors.New("unsupported JSON Schema draft")
	}
	for _, ref := range []string{schema.Ref, schema.DynamicRef} {
		if ref != "" && !strings.HasPrefix(ref, "#") {
			return errors.New("external schema references are disabled")
		}
	}
	for uri, required := range schema.Vocabulary {
		if required && !supportedToolVocabulary(uri) {
			return fmt.Errorf("unsupported required schema vocabulary %q", uri)
		}
	}
	if _, exists := object["multipleOf"]; exists {
		return errors.New("multipleOf exact arithmetic is unsupported by the schema validator")
	}
	if schema.Pattern != "" {
		pattern, err := normalizeToolPattern(schema.Pattern)
		if err != nil {
			return err
		}
		schema.Pattern = pattern
	}
	for _, key := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"} {
		if raw, exists := object[key]; exists {
			number, ok := raw.(json.Number)
			if !ok {
				return fmt.Errorf("%s must be numeric", key)
			}
			rat, ok := new(big.Rat).SetString(number.String())
			if !ok {
				return fmt.Errorf("unsupported numeric bound %s", key)
			}
			f, err := number.Float64()
			converted := new(big.Rat).SetFloat64(f)
			if err != nil || converted == nil || converted.Cmp(rat) != 0 {
				return fmt.Errorf("%s precision is unsupported", key)
			}
			bounds[f] = true
		}
	}
	if value, exists := object["enum"]; exists {
		values, ok := value.([]any)
		if !ok {
			return errors.New("enum must be an array")
		}
		normalized, err := exactToolNumbers(values, nil, true)
		if err != nil {
			return fmt.Errorf("enum precision unsupported: %w", err)
		}
		schema.Enum = normalized.([]any)
	}
	if value, exists := object["const"]; exists {
		normalized, err := exactToolNumbers(value, nil, true)
		if err != nil {
			return fmt.Errorf("const precision unsupported: %w", err)
		}
		schema.Const = &normalized
	}
	// Visit actual subschema fields only; arbitrary objects under enum/default
	// are instance values and must never be mistaken for schema keywords.
	rv := reflect.ValueOf(schema).Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rv.Field(i)
		key := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		raw, exists := object[key]
		if !exists {
			continue
		}
		switch children := field.Interface().(type) {
		case *jsonschema.Schema:
			if children != nil {
				if err := normalizeToolSchema(children, raw, bounds); err != nil {
					return err
				}
			}
		case []*jsonschema.Schema:
			values, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("%s must be an array", key)
			}
			for j, child := range children {
				if err := normalizeToolSchema(child, values[j], bounds); err != nil {
					return err
				}
			}
		case map[string]*jsonschema.Schema:
			values, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s must be an object", key)
			}
			for name, child := range children {
				if err := normalizeToolSchema(child, values[name], bounds); err != nil {
					return err
				}
			}
		}
	}
	// These compatibility fields do not have JSON tags on Schema.
	if raw, exists := object["items"]; exists {
		if schema.Items != nil {
			if err := normalizeToolSchema(schema.Items, raw, bounds); err != nil {
				return err
			}
		}
		if schema.ItemsArray != nil {
			values, ok := raw.([]any)
			if !ok {
				return errors.New("items must be a schema or schema array")
			}
			for i, child := range schema.ItemsArray {
				if err := normalizeToolSchema(child, values[i], bounds); err != nil {
					return err
				}
			}
		}
	}
	if raw, exists := object["dependencies"]; exists && len(schema.DependencySchemas) > 0 {
		values, ok := raw.(map[string]any)
		if !ok {
			return errors.New("dependencies must be an object")
		}
		for name, child := range schema.DependencySchemas {
			if err := normalizeToolSchema(child, values[name], bounds); err != nil {
				return err
			}
		}
	}
	if len(schema.PatternProperties) > 0 {
		patterns := make(map[string]*jsonschema.Schema, len(schema.PatternProperties))
		for pattern, child := range schema.PatternProperties {
			normalized, err := normalizeToolPattern(pattern)
			if err != nil {
				return err
			}
			if _, exists := patterns[normalized]; exists {
				return errors.New("normalized pattern properties are ambiguous")
			}
			patterns[normalized] = child
		}
		schema.PatternProperties = patterns
	}
	return nil
}

func supportedToolVocabulary(uri string) bool {
	switch uri {
	case "https://json-schema.org/draft/2020-12/vocab/core", "https://json-schema.org/draft/2020-12/vocab/applicator", "https://json-schema.org/draft/2020-12/vocab/unevaluated", "https://json-schema.org/draft/2020-12/vocab/validation", "https://json-schema.org/draft/2020-12/vocab/meta-data", "https://json-schema.org/draft/2020-12/vocab/format-annotation", "https://json-schema.org/draft/2020-12/vocab/content":
		return true
	default:
		return false
	}
}
