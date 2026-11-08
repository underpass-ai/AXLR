package application

import (
	"fmt"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A Codex model fills every field of the schema, also those it does not mean.
func TestHostDiscoverReadsACallWithEveryFieldFilled(t *testing.T) {
	snapshot := []domain.AvailableTool{LogsTool()}
	calls := map[string]string{
		"a name beside its query": `{"query":"axlr_logs","name":"axlr_logs","path":"","limit":1,"offset":0}`,
		"a name beside a search":  `{"query":"logs of the run","name":"axlr_logs","path":"","limit":5,"offset":0}`,
	}
	for label, call := range calls {
		result, err := hostDiscover(snapshot, nil, hostJSON(t, call))
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		view, ok := result.(map[string]any)
		if !ok || fmt.Sprint(view["name"]) != "axlr_logs" {
			t.Fatalf("%s: answered %#v, want the exact schema", label, result)
		}
	}
}

func TestHostDiscoverSearchesWhenTheNameIsEmpty(t *testing.T) {
	if _, err := hostDiscover([]domain.AvailableTool{LogsTool()}, nil, hostJSON(t, `{"query":"logs","name":"","path":"","limit":5,"offset":0}`)); err != nil {
		t.Fatal(err)
	}
}

func TestHostDiscoverStillRefusesAPageOfOneTool(t *testing.T) {
	_, err := hostDiscover([]domain.AvailableTool{LogsTool()}, nil, hostJSON(t, `{"name":"axlr_logs","offset":8}`))
	if err == nil || !strings.Contains(err.Error(), "offset is only supported for search") {
		t.Fatalf("err = %v", err)
	}
}
