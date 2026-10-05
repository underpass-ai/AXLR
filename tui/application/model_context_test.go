package application

import (
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
)

func TestModelHostGuidanceBoundsManifestAndKeepsDiscovery(t *testing.T) {
	s := turnSession(t)
	tools := HostTools()
	for i := 0; i < 200; i++ {
		tools = append(tools, hostPlugin(t, fmt.Sprintf("entry_%03d", i), fmt.Sprintf("plugin_%03d", i), "discover_capabilities"))
	}
	if err := s.BeginTurn(root.Text("hello"), tools); err != nil {
		t.Fatal(err)
	}
	guidance := modelHostGuidance(&s)
	if len(guidance.Content) > 13*1024 || !strings.Contains(string(guidance.Content), "omitted") || !strings.Contains(string(guidance.Content), "axlr_tools query/offset") {
		t.Fatal("unbounded or undiscoverable plugin manifest")
	}
	if len(ModelTools(s.ToolSnapshot())) != 8 {
		t.Fatal("plugin definitions leaked")
	}
}

func TestLegacySessionHostUpgradeCannotReplaceAuthority(t *testing.T) {
	s := turnSession(t)
	collision := hostPlugin(t, string(HostCallToolName), "made", "run")
	if err := s.BeginTurn("go", []domain.AvailableTool{collision}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureHostTools(HostTools()); err == nil {
		t.Fatal("host migration replaced existing plugin authority")
	}
	if len(s.ToolSnapshot()) != 1 || s.ToolSnapshot()[0].Identity != collision.Identity {
		t.Fatal("failed migration mutated catalog")
	}
}

func TestModelHostGuidanceDoesNotOfferCatalogueDiscoveryAsEntry(t *testing.T) {
	s := turnSession(t)
	tools := append(HostTools(),
		hostPlugin(t, "made_discover_capabilities", "made", "made_discover_capabilities"),
		hostPlugin(t, "kmp_guide", "kmp", "kmp_guide"))
	if err := s.BeginTurn(root.Text("hello"), tools); err != nil {
		t.Fatal(err)
	}
	guidance := string(modelHostGuidance(&s).Content)
	if strings.Contains(guidance, "made_discover_capabilities") {
		t.Fatal("whole-catalogue discovery offered as an entry name")
	}
	if !strings.Contains(guidance, "Entry names: kmp_guide") {
		t.Fatal("cheap guide entry lost")
	}
}
