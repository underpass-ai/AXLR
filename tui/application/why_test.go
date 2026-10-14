package application

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func whyTool(t *testing.T, name string, schema string) domain.AvailableTool {
	t.Helper()
	parameters, err := root.NewJSONObject([]byte(schema))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := domain.NewLocalToolIdentity(strings.TrimPrefix(name, "local_"))
	return domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: root.ToolName(name), Parameters: parameters}}
}

func TestRequireWhyPatchesOnlyTheSideEffectTools(t *testing.T) {
	exec := whyTool(t, "local_exec", `{"type":"object","properties":{"program":{"type":"string"}},"required":["program"],"additionalProperties":false}`)
	read := whyTool(t, "local_read", `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)
	patched := RequireWhy([]domain.AvailableTool{exec, read})
	var schema struct {
		Properties map[string]struct {
			Type      string `json:"type"`
			MaxLength int    `json:"maxLength"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(patched[0].Definition.Parameters.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["why"].Type != "string" || schema.Properties["why"].MaxLength != WhyMaxRunes || strings.Join(schema.Required, ",") != "program,why" {
		t.Fatalf("exec schema %s", patched[0].Definition.Parameters.Bytes())
	}
	if !RequiresWhy(patched[0]) || RequiresWhy(exec) || RequiresWhy(patched[1]) {
		t.Fatal("RequiresWhy must follow the frozen schema")
	}
	var readSchema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(patched[1].Definition.Parameters.Bytes(), &readSchema); err != nil {
		t.Fatal(err)
	}
	if _, ok := readSchema.Properties["why"]; !ok || strings.Join(readSchema.Required, ",") != "path" {
		t.Fatalf("a read accepts a why without requiring it: %s", patched[1].Definition.Parameters.Bytes())
	}
	// Patching twice changes nothing.
	again := RequireWhy(patched)
	if string(again[0].Definition.Parameters.Bytes()) != string(patched[0].Definition.Parameters.Bytes()) {
		t.Fatal("not idempotent")
	}
}

func TestWhyErrorRefusesAMissingOrOverlongWhyOnlyWhereRequired(t *testing.T) {
	exec := RequireWhy([]domain.AvailableTool{whyTool(t, "local_exec", `{"type":"object","properties":{"program":{"type":"string"}},"required":["program"],"additionalProperties":false}`)})[0]
	if err := whyError(exec, mustObject(t, `{"program":"go"}`)); err == nil || !strings.Contains(err.Error(), MissingWhy) {
		t.Fatalf("%v", err)
	}
	if err := whyError(exec, mustObject(t, `{"program":"go","why":"  "}`)); err == nil {
		t.Fatal("blank why accepted")
	}
	if err := whyError(exec, mustObject(t, `{"program":"go","why":"`+strings.Repeat("x", WhyMaxRunes+1)+`"}`)); err == nil {
		t.Fatal("overlong why accepted")
	}
	if err := whyError(exec, mustObject(t, `{"program":"go","why":"run the tests"}`)); err != nil {
		t.Fatal(err)
	}
	plain := whyTool(t, "local_exec", `{"type":"object","properties":{"program":{"type":"string"}},"required":["program"],"additionalProperties":false}`)
	if err := whyError(plain, mustObject(t, `{"program":"go"}`)); err != nil {
		t.Fatalf("a schema without the requirement asks nothing: %v", err)
	}
}

func TestStripWhyAndCallWhy(t *testing.T) {
	args := mustObject(t, `{"program":"go","args":["test"],"why":" run the tests "}`)
	if CallWhy(args) != "run the tests" {
		t.Fatalf("why %q", CallWhy(args))
	}
	stripped := StripWhy(args)
	if strings.Contains(string(stripped.Bytes()), "why") || !strings.Contains(string(stripped.Bytes()), `"program":"go"`) {
		t.Fatalf("stripped %s", stripped.Bytes())
	}
	without := mustObject(t, `{"program":"go"}`)
	if string(StripWhy(without).Bytes()) != string(without.Bytes()) || CallWhy(without) != "" {
		t.Fatal("arguments without why are returned as they are")
	}
}
