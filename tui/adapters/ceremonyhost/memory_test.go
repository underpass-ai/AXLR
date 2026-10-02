package ceremonyhost

import (
	"strings"
	"testing"
)

func TestWakeProseKeepsOnlyWhatAModelCanUse(t *testing.T) {
	packet := map[string]any{
		"projection": map[string]any{"budget": map[string]any{"max_bytes": 12000}},
		"summary":    "Objective: ws:abc — Memory anchor",
		"wake": map[string]any{
			"current_state": []any{"ws:abc:entry:decision:x (decision): axlr_debug 2.0 ended COMPLETED. Fixed split().", "…"},
			"open_loops":    []any{},
			"next_actions":  []any{"Normalise punctuation"},
		},
	}
	got := wakeProse(packet)
	if got != "- axlr_debug 2.0 ended COMPLETED. Fixed split().\n- Normalise punctuation" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "projection") || wakeProse(map[string]any{}) != "" {
		t.Fatal("envelope leaked or empty packet produced text")
	}
}
