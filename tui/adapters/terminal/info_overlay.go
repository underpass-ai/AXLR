package terminal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func infoContent(state domain.SessionState) string {
	return infoContentLocale(state, English)
}

func infoContentLocale(state domain.SessionState, locale Locale) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\n%s\n%s\n\n%s\n%s\n\n%s", Translate(locale, "info.model"), state.Model, Translate(locale, "info.workspace"), state.Workspace, Translate(locale, "info.session"), state.ID, Translate(locale, "info.fullResults"))
	if len(state.Activity) == 0 {
		b.WriteString("\n" + Translate(locale, "info.noTools"))
	}
	for _, record := range state.Activity {
		if record.Outcome == nil {
			continue
		}
		label, _ := toolPresentation(state, record.Call.Name)
		fmt.Fprintf(&b, "\n\n%s · %s\n\n%s", label, Translate(locale, "decision."+string(record.Decision)), prettyToolResult(string(record.Outcome.Content)))
	}
	return b.String()
}

func prettyToolResult(value string) string {
	if len(value) > 128<<10 || !json.Valid([]byte(value)) {
		return value
	}
	var formatted bytes.Buffer
	if json.Indent(&formatted, []byte(value), "", "  ") != nil {
		return value
	}
	return formatted.String()
}
