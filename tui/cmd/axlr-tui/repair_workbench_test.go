package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/axlrplugin"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestRepairWorkbenchesOpenARuntimeRootedInTheClone(t *testing.T) {
	registrations := []plugins.Registration{{Manifest: plugins.Manifest{ID: root.PluginID("made"), Command: filepath.Join(t.TempDir(), "made-mcp"), AllowAll: true}}}
	manager, err := plugins.NewManager(registrations)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	benches := repairWorkbenches{env: []string{"PATH=/usr/bin:/bin"}, manager: manager, registrations: registrations, catalog: &axlrplugin.Catalog{Root: t.TempDir()}, configPath: filepath.Join(t.TempDir(), "mcp.json"), getenv: func(string) string { return "" }, autonomous: true}
	clone := t.TempDir()
	workbench, err := benches.Open(context.Background(), clone)
	if err != nil {
		t.Fatal(err)
	}
	useCases, ok := workbench.(*application.UseCaseWorkbench)
	if !ok || useCases.Start.Continue.Ceremonies == nil || useCases.Start.Continue.Ceremonies.Forge == nil || useCases.Start.Approval == nil {
		t.Fatalf("workbench not wired: %+v", workbench)
	}
	exec, err := domain.NewLocalToolIdentity("exec")
	if err != nil {
		t.Fatal(err)
	}
	if !useCases.Start.Approval.AutoApproves(exec) {
		t.Fatal("an autonomous repair session approves its own local commands")
	}
	if err := workbench.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := benches.Open(context.Background(), filepath.Join(clone, "missing")); err == nil {
		t.Fatal("a missing clone opened")
	}
	withoutMADE, err := plugins.NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer withoutMADE.Close()
	benches.manager, benches.registrations = withoutMADE, nil
	if _, err := benches.Open(context.Background(), clone); err == nil || !strings.Contains(err.Error(), "MADE is not connected") {
		t.Fatalf("without MADE: %v", err)
	}
	if repairPanelPort(nil) != nil || repairPanelPort(&application.SelfRepair{}) == nil {
		t.Fatal("panel port")
	}
}
