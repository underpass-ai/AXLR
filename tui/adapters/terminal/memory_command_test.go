package terminal

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

func TestMemoryCompareContentListsAboutsRelationsAndSaysNothingWasWritten(t *testing.T) {
	theme := Theme{Monochrome: true}
	result := application.CompareResult{Abouts: []string{"project:A", "ws:1"}, Facts: map[string]int{"project:A": 2, "ws:1": 1},
		Proposals: []application.MemoryRelation{{From: "project:A:entry:decision:x", To: "ws:1:entry:turn:t", Rel: "before", Dimension: "session", Scope: "1", Why: "Both stand in session=1"}}}
	text := memoryCompareContent(result, theme)
	for _, part := range []string{"Compared: project:A (2 entries) · ws:1 (1 entry)", "Proposals from shared coordinates", "- project:A:entry:decision:x\n    before ws:1:entry:turn:t · session=1\n    Both stand in session=1", "Nothing was written."} {
		if !strings.Contains(text, part) {
			t.Fatalf("content lacks %q:\n%s", part, text)
		}
	}
	empty := memoryCompareContent(application.CompareResult{Abouts: []string{"project:A", "ws:1"}, Facts: map[string]int{}}, theme)
	if !strings.Contains(empty, "KMP declares no relation and proposes none") {
		t.Fatalf("empty:\n%s", empty)
	}
	if usage := slashUsage("/memory"); !strings.Contains(usage, "save") || !strings.Contains(usage, "forget <ref>") {
		t.Fatalf("usage %q", usage)
	}
	if usage := slashUsage("/session"); usage != "/session [export [id]]" {
		t.Fatalf("usage %q", usage)
	}
}
