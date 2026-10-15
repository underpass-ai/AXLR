package application

import (
	"encoding/json"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
)

// A host tool the session offers is found by its own name (seen on 15 and
// 16 Oct 2026: Haiku searched axlr_tools for "axlr_logs", got no match and
// asked for a repair).
func TestAxlrToolsSearchFindsAxlrLogs(t *testing.T) {
	result, err := hostDiscover([]domain.AvailableTool{LogsTool()}, nil, hostJSON(t, `{"query":"axlr_logs"}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	var got struct {
		Tools []map[string]any `json:"tools"`
		Total int              `json:"total_matches"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil || got.Total != 1 || len(got.Tools) != 1 || got.Tools[0]["name"] != "axlr_logs" || got.Tools[0]["host"] != true || got.Tools[0]["call_directly"] != true {
		t.Fatalf("%s %v", encoded, err)
	}
}
