package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolSummary bounds rendering work; the complete result stays in session history.
func toolSummary(text string) string {
	const limit = 160
	short := text
	if len(short) > 1024 {
		short = short[:1024]
	}
	short = strings.Join(strings.Fields(Sanitize(short)), " ")
	short = ansi.Truncate(short, limit, "…")
	if len(text) > len(short) {
		short += fmt.Sprintf(" [%d bytes; full result saved]", len(text))
	}
	return short
}

// Historical tool names are display metadata only. Matching their legacy alias
// here cannot authorize them for execution against a new catalog snapshot.
func toolPresentation(s domain.SessionState, name root.ToolName) (string, bool) {
	for _, tool := range s.ToolSnapshot {
		if tool.Identity.Kind != domain.ToolKindPlugin {
			continue
		}
		ref := tool.Identity.Plugin
		matches := tool.Definition.Name == name
		if !matches && strings.HasPrefix(string(name), "mcp_") {
			pair, _ := json.Marshal([2]string{ref.PluginID.String(), ref.ToolName.String()})
			digest := sha256.Sum256(pair)
			matches = string(name) == "mcp_"+hex.EncodeToString(digest[:24])
		}
		if matches {
			return string(ref.PluginID) + " / " + string(ref.ToolName), ref.PluginID == "kmp"
		}
	}
	return string(name), false
}
