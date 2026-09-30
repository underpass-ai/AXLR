package domain

import (
	root "github.com/underpass-ai/AXLR/domain"
	"testing"
)

func TestHostToolIdentityIsSeparateFromPluginAndLocalAuthority(t *testing.T) {
	for _, operation := range []string{HostOperationTools, HostOperationCallTool, HostOperationHistory} {
		id, err := NewHostToolIdentity(operation)
		if err != nil || id.Kind != ToolKindHost || id.LocalOperation != operation {
			t.Fatalf("%+v %v", id, err)
		}
		if _, err := NewLocalToolIdentity(operation); err == nil {
			t.Fatal("host operation gained local authority")
		}
		id.Plugin = root.PluginRef{PluginID: "kmp", ToolName: "kmp_wake"}
		if err := id.Validate(); err == nil {
			t.Fatal("host identity carries plugin privilege")
		}
	}
	for _, operation := range []string{"read", "exec", "", "remove"} {
		if _, err := NewHostToolIdentity(operation); err == nil {
			t.Fatalf("invalid host operation %q", operation)
		}
	}
	local, err := NewLocalToolIdentity("exec")
	if err != nil || local.Validate() != nil {
		t.Fatal("local authority changed")
	}
	plugin, err := NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_wake"})
	if err != nil || plugin.Validate() != nil {
		t.Fatal("plugin authority changed")
	}
}
