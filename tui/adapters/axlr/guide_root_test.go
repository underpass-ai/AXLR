package axlr

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// KMP's answer for a store without a guide names the sync command with a
// <plugin-root> placeholder; the console fills in the engine it launched.
func TestTheKMPGuideErrorNamesThePluginRoot(t *testing.T) {
	message := "Run `kmp-mcp guide sync --plugin-root <plugin-root>`, replacing <plugin-root> with the matching plugin directory."
	encoded, _ := json.Marshal(map[string]any{"output": map[string]any{"content": []any{map[string]any{"type": "text", "text": message}}, "structured_content": map[string]any{"repair": map[string]any{"arguments": []string{"guide", "sync", "--plugin-root", "<plugin-root>"}}}}})
	kmp, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: "kmp_guide"})
	other, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "github", ToolName: "search"})
	runner := ToolRunner{KMPGuideRoot: `/home/me/.local/share/axlr/engines/kmp-0.25.0`}
	got := runner.namedGuideRoot(kmp, string(encoded))
	if strings.Contains(got, "plugin-root\\u003e") || strings.Count(got, "/home/me/.local/share/axlr/engines/kmp-0.25.0") != 3 || !json.Valid([]byte(got)) {
		t.Fatalf("filled: %s", got)
	}
	if runner.namedGuideRoot(other, string(encoded)) != string(encoded) || (ToolRunner{}).namedGuideRoot(kmp, string(encoded)) != string(encoded) {
		t.Fatal("another plugin's answer, or one without a known root, changed")
	}
}
