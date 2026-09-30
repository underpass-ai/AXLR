package axlr

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

var _ application.ToolArgumentValidationPort = (*ToolArgumentValidator)(nil)

func validationSchema(t *testing.T, text string) root.ToolDefinition {
	t.Helper()
	value, err := root.NewJSONValue([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return root.ToolDefinition{Name: "memory", Parameters: value}
}

func TestToolArgumentValidatorChecksRealPluginSchemaBeforeEffects(t *testing.T) {
	validator := NewToolArgumentValidator()
	schema := validationSchema(t, `{"type":"object","properties":{"about":{"type":"string","minLength":1},"mode":{"type":"string","enum":["read","write"]},"count":{"type":"integer","minimum":0,"maximum":8}},"required":["about","mode"],"additionalProperties":false}`)
	if err := validator.Validate(schema, jsonValue(t, `{"about":"project:AXLR","mode":"read","count":4}`)); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{}`, `{"about":3,"mode":"read"}`, `{"about":"","mode":"read"}`, `{"about":"x","mode":"remove"}`, `{"about":"x","mode":"read","extra":true}`, `{"about":"x","mode":"read","count":1.5}`, `{"about":"x","mode":"read","count":9}`, `{"about":"x","mode":"read","count":-1}`} {
		if err := validator.Validate(schema, jsonValue(t, args)); err == nil {
			t.Fatalf("invalid actual target arguments accepted: %s", args)
		}
	}
	if len(validator.schemas) != 1 {
		t.Fatal("schema not cached")
	}
}

func TestToolArgumentValidatorSupportsRegisteredMADEUnicodePattern(t *testing.T) {
	validator := NewToolArgumentValidator()
	schema := validationSchema(t, `{"type":"object","properties":{"name":{"type":"string","pattern":"^[^\\x00-\\x1F\\x7F\\u0080-\\u009F]*[^\\s\\x00-\\x1F\\x7F\\u0080-\\u009F][^\\x00-\\x1F\\x7F\\u0080-\\u009F]*$"}},"required":["name"]}`)
	for _, args := range []string{`{"name":"Design"}`, `{"name":"Diseño de ceremonia"}`, `{"name":"\u00a0Design\u00a0"}`} {
		if err := validator.Validate(schema, jsonValue(t, args)); err != nil {
			t.Fatal("valid MADE name rejected", err)
		}
	}
	for _, args := range []string{`{"name":"\u0080Design"}`, `{"name":"\u009f"}`, `{"name":"\u00a0"}`, `{"name":"\u2028"}`, `{"name":"  "}`} {
		if err := validator.Validate(schema, jsonValue(t, args)); err == nil {
			t.Fatal("invalid MADE name accepted", args)
		}
	}
	literal := validationSchema(t, `{"type":"object","properties":{"name":{"type":"string","pattern":"^\\\\u0080$"}}}`)
	if err := validator.Validate(literal, jsonValue(t, `{"name":"\\u0080"}`)); err != nil {
		t.Fatal("escaped backslash changed", err)
	}
	patternProperty := validationSchema(t, `{"type":"object","patternProperties":{"^\\u0080$":{"type":"integer"}},"additionalProperties":false}`)
	if err := validator.Validate(patternProperty, jsonValue(t, `{"\u0080":1}`)); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{`\uD800`, `\u{0080}`, `\u12`, `[\S]`} {
		if _, err := normalizeToolPattern(pattern); err == nil {
			t.Fatal("unsupported escape silently accepted", pattern)
		}
	}
}

func TestToolArgumentValidatorPreservesExactNumbersIncludingSchemaEnums(t *testing.T) {
	validator := NewToolArgumentValidator()
	for _, constraint := range []string{`"enum":[9007199254740993]`, `"const":9007199254740993`} {
		schema := validationSchema(t, `{"type":"object","properties":{"n":{"type":"integer",`+constraint+`}},"required":["n"]}`)
		if err := validator.Validate(schema, jsonValue(t, `{"n":9007199254740993}`)); err != nil {
			t.Fatalf("exact valid integer changed: %v", err)
		}
		if err := validator.Validate(schema, jsonValue(t, `{"n":9007199254740992}`)); err == nil {
			t.Fatal("rounded neighboring integer admitted")
		}
	}
	schema := validationSchema(t, `{"type":"object","properties":{"value":{"enum":[{"array":[9007199254740993]}]}}}`)
	if err := validator.Validate(schema, jsonValue(t, `{"value":{"array":[9007199254740993]}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(schema, jsonValue(t, `{"value":{"array":[9007199254740992]}}`)); err == nil {
		t.Fatal("nested enum numbers rounded")
	}
	schema = validationSchema(t, `{"type":"object","properties":{"n":{"type":"integer","minimum":9007199254740992}}}`)
	if err := validator.Validate(schema, jsonValue(t, `{"n":9007199254740993}`)); err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(schema, jsonValue(t, `{"n":9007199254740991}`)); err == nil {
		t.Fatal("exact bound comparison failed")
	}
	decimal := validationSchema(t, `{"type":"object","properties":{"n":{"type":"number","minimum":0,"maximum":1}}}`)
	for _, value := range []string{"0.8", "0.1", "0.5"} {
		if err := validator.Validate(decimal, jsonValue(t, `{"n":`+value+`}`)); err != nil {
			t.Fatal("ordinary decimal rejected", value, err)
		}
	}
	for _, value := range []string{"1.00000000000000000001", "-0.00000000000000000001"} {
		if err := validator.Validate(decimal, jsonValue(t, `{"n":`+value+`}`)); err == nil {
			t.Fatal("decimal rounding evaded bound", value)
		}
	}
	integer := validationSchema(t, `{"type":"object","properties":{"n":{"type":"integer"}}}`)
	if err := validator.Validate(integer, jsonValue(t, `{"n":1.00000000000000000001}`)); err == nil {
		t.Fatal("fraction rounded to integer")
	}
}

func TestToolArgumentValidatorSupportsNestedSchemasAndLocalReferences(t *testing.T) {
	validator := NewToolArgumentValidator()
	schema := validationSchema(t, `{"type":"object","$defs":{"positive":{"type":"integer","minimum":1}},"properties":{"values":{"type":"array","items":{"$ref":"#/$defs/positive"},"minItems":1},"flag":{"oneOf":[{"const":true},{"const":false}]},"data":{"additionalProperties":{"enum":[9007199254740993]}}},"required":["values","flag"]}`)
	if err := validator.Validate(schema, jsonValue(t, `{"values":[1,2],"flag":true,"data":{"x":9007199254740993}}`)); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"values":[0],"flag":true}`, `{"values":[],"flag":true}`, `{"values":[1],"flag":"yes"}`, `{"values":[1],"flag":true,"data":{"x":9007199254740992}}`} {
		if err := validator.Validate(schema, jsonValue(t, args)); err == nil {
			t.Fatal("invalid nested arguments admitted", args)
		}
	}
	legacy := validationSchema(t, `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"tuple":{"type":"array","items":[{"enum":[9007199254740993]}]},"anchor":{"type":"string"}},"dependencies":{"anchor":{"properties":{"n":{"const":9007199254740993}},"required":["n"]}}}`)
	if err := validator.Validate(legacy, jsonValue(t, `{"tuple":[9007199254740993],"anchor":"x","n":9007199254740993}`)); err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(legacy, jsonValue(t, `{"tuple":[9007199254740992]}`)); err == nil {
		t.Fatal("legacy tuple exact enum lost")
	}
	if err := validator.Validate(legacy, jsonValue(t, `{"anchor":"x","n":9007199254740992}`)); err == nil {
		t.Fatal("legacy dependency exact const lost")
	}
}

func TestToolArgumentValidatorRejectsUnsupportedOrMalformedSchemas(t *testing.T) {
	validator := NewToolArgumentValidator()
	for _, schema := range []string{
		`null`, `true`, `[]`, `{"type":"imaginary"}`, `{"type":"object","required":"about"}`, `{"type":"object","properties":{"x":null}}`,
		`{"type":"object","$ref":"https://127.0.0.1/should-not-be-loaded"}`, `{"type":"object","$ref":"file:///etc/passwd"}`, `{"type":"object","$dynamicRef":"schema.json#x"}`,
		`{"$schema":"https://json-schema.org/draft/2019-09/schema","type":"object"}`, `{"type":"object","unsupportedAssertion":true}`, `{"type":"object","properties":{"x":{"unknown":true}}}`,
		`{"type":"object","$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/format-assertion":true}}`,
		`{"type":"object","properties":{"n":{"minimum":9007199254740993}}}`, `{"type":"object","properties":{"n":{"minimum":0.1}}}`, `{"type":"object","properties":{"n":{"multipleOf":2}}}`,
		`{"type":"object","type":"string"}`, `{"type":"object","properties":{"x":{"enum":"wrong"}}}`, `{"type":"object","properties":{"x":{"minimum":"zero"}}}`,
		`{"type":"object","$ref":"#"}`, `{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`, `{"type":"object","properties":{"next":{"$ref":"#"}}}`, `{"type":"object","properties":{"x":{"enum":[0.8]}}}`,
	} {
		if err := validator.Validate(validationSchema(t, schema), jsonValue(t, `{}`)); err == nil {
			t.Fatalf("unsupported/malformed schema admitted: %s", schema)
		}
	}
	if err := validator.Validate(root.ToolDefinition{}, jsonValue(t, `{}`)); err == nil {
		t.Fatal("missing schema admitted")
	}
	if err := validator.Validate(validationSchema(t, `{"type":"object","description":"`+strings.Repeat("z", maxToolSchemaBytes)+`"}`), jsonValue(t, `{}`)); err == nil {
		t.Fatal("oversized schema admitted")
	}
	var absent *ToolArgumentValidator
	if err := absent.Validate(validationSchema(t, `{}`), jsonValue(t, `{}`)); err == nil {
		t.Fatal("missing validator admitted")
	}
}

func TestToolArgumentValidatorAnnotationsAndDuplicateArgumentKeys(t *testing.T) {
	validator := NewToolArgumentValidator()
	schema := validationSchema(t, `{"type":"object","x-made-pattern-catalog":{"pattern":"guide only"},"properties":{"about":{"type":"string","format":"uri"}}}`)
	if err := validator.Validate(schema, jsonValue(t, `{"about":"project:AXLR"}`)); err != nil {
		t.Fatal("annotation treated as assertion", err)
	}
	for _, args := range []string{`{"about":"x","about":"y"}`, `{"object":{"nested":1,"nested":2}}`} {
		if err := validator.Validate(schema, jsonValue(t, args)); err == nil {
			t.Fatal("duplicate argument key admitted")
		}
	}
	value, _ := root.NewJSONValue([]byte(`[]`))
	if err := validator.Validate(schema, value); err == nil {
		t.Fatal("nonobject plugin arguments")
	}
	deep := `{"nested":` + strings.Repeat(`[`, 129) + `0` + strings.Repeat(`]`, 129) + `}`
	if err := validator.Validate(schema, jsonValue(t, deep)); err == nil {
		t.Fatal("unbounded nesting")
	}
}

func TestToolArgumentValidatorCacheIsBoundedConcurrentAndTracksSchemaChanges(t *testing.T) {
	validator := NewToolArgumentValidator()
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 20; j++ {
				schema := validationSchema(t, `{"type":"object","properties":{"n":{"enum":[9007199254740993]}}}`)
				if err := validator.Validate(schema, jsonValue(t, `{"n":9007199254740993}`)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	workers.Wait()
	for i := 0; i < maxCompiledToolSchemas+20; i++ {
		schema := validationSchema(t, fmt.Sprintf(`{"type":"object","title":"schema-%d"}`, i))
		if err := validator.Validate(schema, jsonValue(t, `{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if len(validator.schemas) != maxCompiledToolSchemas || len(validator.order) != maxCompiledToolSchemas {
		t.Fatal("unbounded compiled schema cache")
	}
	changed := validationSchema(t, `{"type":"object","required":["new_required"]}`)
	if err := validator.Validate(changed, jsonValue(t, `{}`)); err == nil {
		t.Fatal("stale schema cache accepted missing required property")
	}
}
