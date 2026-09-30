package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolSummary bounds rendering work; the complete result stays in session history.
func toolSummary(text string) string {
	return toolSummaryLocale(text, English)
}

func toolSummaryLocale(text string, locale Locale) string {
	const limit = 96
	short := text
	if len(short) > 1024 {
		short = short[:1024]
	}
	short = strings.Join(strings.Fields(Sanitize(short)), " ")
	short = ansi.Truncate(short, limit, "…")
	if len(text) > len(short) {
		short += Translatef(locale, "transcript.savedBytes", len(text))
	}
	return short
}

func toolResultSummary(text string) string {
	return toolResultSummaryLocale(text, English)
}

func toolResultSummaryLocale(text string, locale Locale) string {
	if len(text) <= 4<<10 {
		var result struct {
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(text), &result) == nil && result.Status != "" {
			status := ansi.Truncate(singleLine(result.Status), 30, "…")
			return Translatef(locale, "transcript.savedResult", status, len(text))
		}
	}
	return toolSummaryLocale(text, locale)
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
