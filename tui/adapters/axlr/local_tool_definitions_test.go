package axlr

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	axlrruntime "github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func localDefinition(t *testing.T, name root.ToolName) root.ToolDefinition {
	t.Helper()
	for _, tool := range localToolDefinitions() {
		if tool.Definition.Name == name {
			return tool.Definition
		}
	}
	t.Fatalf("%s is not defined", name)
	return root.ToolDefinition{}
}

// On 10 October 2026 claude-haiku-5.5 sent local_exec timeout_ms 600000,
// which the schema allowed; the person approved the card and the runtime
// refused the call. The schema now states the runtime's limits.
func TestLocalExecSchemaStatesTheRuntimeLimits(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Maximum *int64 `json:"maximum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(localDefinition(t, "local_exec").Parameters.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties["timeout_ms"].Maximum; got == nil || *got != axlrruntime.HardTimeout.Milliseconds() {
		t.Fatalf("timeout_ms maximum %v, want %d", got, axlrruntime.HardTimeout.Milliseconds())
	}
	if got := schema.Properties["max_output_bytes"].Maximum; got == nil || *got != int64(axlrruntime.HardOutputBytes) {
		t.Fatalf("max_output_bytes maximum %v, want %d", got, axlrruntime.HardOutputBytes)
	}
}

// The console validates local arguments against these schemas before the
// approval card, so every one must compile in the validator.
func TestLocalToolSchemasCompileAndBoundExec(t *testing.T) {
	validator := NewToolArgumentValidator()
	empty, _ := root.NewJSONObject([]byte(`{}`))
	for _, tool := range localToolDefinitions() {
		if err := validator.Validate(tool.Definition, empty); err != nil && strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("%s: %v", tool.Definition.Name, err)
		}
	}
	exec := localDefinition(t, "local_exec")
	over, _ := root.NewJSONObject([]byte(`{"program":"go","timeout_ms":600000}`))
	if err := validator.Validate(exec, over); err == nil {
		t.Fatal("timeout_ms 600000 accepted")
	}
	within, _ := root.NewJSONObject([]byte(`{"program":"go","timeout_ms":300000,"max_output_bytes":1048576}`))
	if err := validator.Validate(exec, within); err != nil {
		t.Fatal(err)
	}
}

// The console's catalog requires a why on every side effect; the service's
// catalog, which API clients read, keeps the plain schemas.
func TestTheCatalogRequiresAWhyOnlyWhenAsked(t *testing.T) {
	plain, err := (ToolCatalog{}).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	required, err := (ToolCatalog{RequireWhy: true}).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	find := func(tools []domain.AvailableTool, name string) string {
		for _, tool := range tools {
			if string(tool.Definition.Name) == name {
				return string(tool.Definition.Parameters.Bytes())
			}
		}
		t.Fatalf("%s missing", name)
		return ""
	}
	parse := func(schema string) (hasWhy, requiresWhy bool) {
		var object struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal([]byte(schema), &object); err != nil {
			t.Fatal(err)
		}
		_, hasWhy = object.Properties["why"]
		for _, name := range object.Required {
			requiresWhy = requiresWhy || name == "why"
		}
		return hasWhy, requiresWhy
	}
	for _, name := range []string{"local_write", "local_edit", "local_exec", "axlr_call_tool"} {
		if has, _ := parse(find(plain, name)); has {
			t.Fatalf("%s mentions a why without being asked", name)
		}
		if has, requires := parse(find(required, name)); !has || !requires {
			t.Fatalf("%s lacks the why requirement: %s", name, find(required, name))
		}
	}
	for _, name := range []string{"local_read", "local_search", "local_list"} {
		if has, requires := parse(find(required, name)); !has || requires {
			t.Fatalf("%s must accept a why without requiring it: %s", name, find(required, name))
		}
	}
	if strings.Contains(find(required, "axlr_tools"), `"why"`) {
		t.Fatal("axlr_tools must not mention a why")
	}
}
