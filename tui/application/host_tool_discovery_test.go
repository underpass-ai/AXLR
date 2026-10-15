package application

import (
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
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

// An exact name returns a host tool's schema, or the part path selects,
// marked to be called by its own name.
func TestAxlrToolsDescribesAHostToolByName(t *testing.T) {
	snapshot := []domain.AvailableTool{LogsTool()}
	result, err := hostDiscover(snapshot, nil, hostJSON(t, `{"name":"axlr_logs"}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if text := string(encoded); !strings.Contains(text, `"call_directly":true`) || !strings.Contains(text, `"host":true`) || !strings.Contains(text, `"all_consoles"`) || strings.Contains(text, `"plugin"`) {
		t.Fatalf("%s", encoded)
	}
	result, err = hostDiscover(snapshot, nil, hostJSON(t, `{"name":"axlr_logs","path":"/properties/level"}`))
	if encoded, _ := json.Marshal(result); err != nil || !strings.Contains(string(encoded), `"WARN"`) {
		t.Fatalf("%s %v", encoded, err)
	}
}

// A word the session's own tools happen to share is no reason to list them:
// they come back only when every term matches, never as partial matches.
func TestAxlrToolsListsTheSessionsToolsOnlyOnAFullMatch(t *testing.T) {
	snapshot := append(HostTools(), LogsTool())
	result, err := hostDiscover(snapshot, nil, hostJSON(t, `{"query":"console nonexistent"}`))
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(result); !strings.Contains(string(encoded), `"total_matches":0`) {
		t.Fatalf("%s", encoded)
	}
}

// Discovery lists the local and host tools the session is offered, not the
// whole snapshot: review mode hides local_write, a repair session never
// sees axlr_request_repair, and axlr_step_done needs a ceremony step.
func TestAxlrToolsListsOnlyTheToolsTheSessionOffers(t *testing.T) {
	write, err := domain.NewLocalToolIdentity("write")
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	tools := append(HostTools(), LogsTool(), domain.AvailableTool{Identity: write, Definition: root.ToolDefinition{Name: "local_write", Description: "Write a file", Parameters: schema}})
	for _, tc := range []struct {
		mode    domain.WorkMode
		listed  []string
		omitted []string
	}{
		{domain.ModeNormal, []string{"local_write", "axlr_request_repair", "axlr_logs"}, []string{"axlr_step_done"}},
		{domain.ModeReview, []string{"axlr_request_repair", "axlr_logs"}, []string{"local_write", "axlr_step_done"}},
		{domain.ModeRepair, []string{"local_write", "axlr_logs"}, []string{"axlr_request_repair", "axlr_request_improvement", "axlr_step_done"}},
	} {
		s := turnSession(t)
		if err := s.SetMode(tc.mode); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginTurn("user instruction", tools); err != nil {
			t.Fatal(err)
		}
		listing := string(hostExecute(t, s, domain.HostOperationTools, `{"limit":20}`).Content)
		for _, name := range tc.listed {
			if !strings.Contains(listing, `"name":"`+name+`"`) {
				t.Fatalf("%s: %s not listed: %s", tc.mode, name, listing)
			}
		}
		for _, name := range tc.omitted {
			if strings.Contains(listing, `"name":"`+name+`"`) {
				t.Fatalf("%s: %s listed: %s", tc.mode, name, listing)
			}
			if out := hostExecute(t, s, domain.HostOperationTools, `{"name":"`+name+`"}`); !out.IsError {
				t.Fatalf("%s: %s described: %s", tc.mode, name, out.Content)
			}
		}
	}
}
