package axlr

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"path/filepath"
	"testing"
)

func pluginProfile(id string, mode domain.ApprovalMode) domain.PluginProfile {
	return domain.PluginProfile{ID: root.PluginID(id), Name: root.Text(id), Purpose: domain.PluginPurposeTools, Approval: mode}
}
func pluginIdentity(id, tool string) domain.ToolIdentity {
	value, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: root.PluginID(id), ToolName: root.PluginToolName(tool)})
	return value
}
func TestPluginManagerPolicyUsesExactIdentityAndPersistence(t *testing.T) {
	profiles := []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalAuto), pluginProfile("beta", domain.ApprovalManual)}
	calls := 0
	manager := NewPluginManager(nil, profiles, func(_ context.Context, id root.PluginID, mode domain.ApprovalMode) error {
		calls++
		if id != "beta" || mode != domain.ApprovalAuto {
			t.Fatalf("incorrect persisted policy: %s %s", id, mode)
		}
		return nil
	})
	profiles[0].Approval = domain.ApprovalManual
	snapshot := manager.Profiles()
	snapshot[0].Approval = domain.ApprovalManual
	if !manager.AutoApproves(pluginIdentity("alpha", "read")) {
		t.Fatal("profile slices were shared")
	}
	for _, identity := range []domain.ToolIdentity{pluginIdentity("beta", "alpha"), pluginIdentity("alpha_extra", "read"), {Kind: domain.ToolKindLocal, LocalOperation: "exec"}, {Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: "alpha"}}, {Kind: domain.ToolKindPlugin, LocalOperation: "read", Plugin: root.PluginRef{PluginID: "alpha", ToolName: "read"}}} {
		if manager.AutoApproves(identity) {
			t.Fatalf("autoapproved unsupported identity %+v", identity)
		}
	}
	if err := manager.SetApproval(context.Background(), "beta", domain.ApprovalAuto); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !manager.AutoApproves(pluginIdentity("beta", "echo")) {
		t.Fatal("policy did not update after persistence")
	}
}
func TestPluginManagerPersistenceFailureRetainsManualPolicy(t *testing.T) {
	manager := NewPluginManager(nil, []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalManual)}, func(context.Context, root.PluginID, domain.ApprovalMode) error { return errors.New("cannot persist") })
	if err := manager.SetApproval(context.Background(), "alpha", domain.ApprovalAuto); err == nil || manager.AutoApproves(pluginIdentity("alpha", "echo")) {
		t.Fatal("unpersisted approval applied")
	}
	manager = NewPluginManager(nil, []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalManual)}, nil)
	for _, change := range []struct {
		id   root.PluginID
		mode domain.ApprovalMode
	}{{"alpha", domain.ApprovalAuto}, {"unknown", domain.ApprovalManual}, {"bad/id", domain.ApprovalManual}, {"alpha", "invalid"}} {
		if err := manager.SetApproval(context.Background(), change.id, change.mode); err == nil {
			t.Fatal("invalid policy change accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if manager.SetApproval(ctx, "alpha", domain.ApprovalAuto) == nil {
		t.Fatal("cancelled change applied")
	}
	invalid := pluginProfile("alpha", domain.ApprovalAuto)
	invalid.Purpose = "invalid"
	if NewPluginManager(nil, []domain.PluginProfile{invalid}, nil).AutoApproves(pluginIdentity("alpha", "echo")) {
		t.Fatal("invalid profile granted authority")
	}
}
func TestPluginManagerListsToolsWithoutChangingAuthorization(t *testing.T) {
	manager := NewPluginManager(testManager(t, ""), []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalAuto), pluginProfile("beta", domain.ApprovalManual)}, nil)
	states, err := manager.List(context.Background())
	if err != nil || len(states) != 2 {
		t.Fatalf("discovery: %+v %v", states, err)
	}
	for _, state := range states {
		if state.Error != "" || len(state.Tools) != 1 || state.Tools[0] != "echo" {
			t.Fatalf("wrong state: %+v", state)
		}
	}
	if !manager.AutoApproves(pluginIdentity("alpha", "echo")) || manager.AutoApproves(pluginIdentity("beta", "echo")) {
		t.Fatal("discovery changed policy")
	}
	empty, err := NewPluginManager(nil, nil, nil).List(context.Background())
	if err != nil || len(empty) != 0 {
		t.Fatal("empty manager failed")
	}
	states, err = NewPluginManager(nil, manager.Profiles(), nil).List(context.Background())
	if err != nil || states[0].Error == "" {
		t.Fatal("unavailable status absent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
func TestPluginManagerDiscoveryFailureHasSafeStatus(t *testing.T) {
	raw := testManager(t, "")
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	states, err := NewPluginManager(raw, []domain.PluginProfile{pluginProfile("alpha", domain.ApprovalManual)}, nil).List(context.Background())
	if err != nil || len(states) != 1 || states[0].Error != "MCP discovery failed; check the configured server" {
		t.Fatalf("unsafe discovery failure: %+v %v", states, err)
	}
}

func TestPluginManagerMixedDiscoveryPreservesHealthyServerStatus(t *testing.T) {
	healthy, err := plugins.NewRegistration(plugins.Manifest{ID: "alpha", Command: os.Args[0], Args: []string{"-test.run=^TestMCPHelper$"}, AllowTools: []root.PluginToolName{"echo"}}, []string{"AXLR_TUI_HELPER=1", "MARKER=", "IDENTITY=alpha"})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := plugins.NewRegistration(plugins.Manifest{ID: "beta", Command: filepath.Join(t.TempDir(), "missing-server"), AllowTools: []root.PluginToolName{"echo"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := plugins.NewManager([]plugins.Registration{failed, healthy})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	manager := NewPluginManager(raw, []domain.PluginProfile{pluginProfile("beta", domain.ApprovalManual), pluginProfile("alpha", domain.ApprovalAuto)}, nil)
	states, err := manager.List(context.Background())
	if err != nil || len(states) != 2 {
		t.Fatalf("discovery: %+v %v", states, err)
	}
	if states[0].Error != "MCP discovery failed; check the configured server" || len(states[0].Tools) != 0 {
		t.Fatalf("failed server status: %+v", states[0])
	}
	if states[1].Error != "" || len(states[1].Tools) != 1 || states[1].Tools[0] != "echo" {
		t.Fatalf("healthy server incorrectly unavailable: %+v", states[1])
	}
	if !manager.AutoApproves(pluginIdentity("alpha", "echo")) || manager.AutoApproves(pluginIdentity("beta", "echo")) {
		t.Fatal("discovery altered authorization")
	}
}
