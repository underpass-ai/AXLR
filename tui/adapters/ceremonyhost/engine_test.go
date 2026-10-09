package ceremonyhost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// madeAnswers answers MADE tools by name: a structured result, or a failed
// envelope with the message when the answer is an error.
func madeAnswers(answers map[string]any) memoryToolsFunc {
	return func(_ context.Context, id domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		answer := answers[string(id.Plugin.ToolName)]
		if err, failed := answer.(error); failed {
			encoded, _ := json.Marshal(map[string]any{"status": "failed", "error": map[string]any{"message": err.Error()}})
			return domain.ToolOutcome{Content: root.Text(encoded), IsError: true}, nil
		}
		encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(encoded)}, nil
	}
}

// A failed made_inspect_ceremony_resume used to be dropped: the view then
// said "no live claim of ours", and nothing showed MADE's error.
func TestEngineInspectSurfacesAFailedResumeInspection(t *testing.T) {
	engine := Engine{Tools: madeAnswers(map[string]any{
		"made_get_ceremony_instance":   map[string]any{"current_state": "PROPOSE", "claimable_step_ids": []any{}},
		"made_inspect_ceremony_resume": fmt.Errorf("journal is being compacted"),
	})}
	view, err := engine.Inspect(context.Background(), "axlr-x-1")
	if err != nil || view.State != "PROPOSE" {
		t.Fatalf("the instance itself was read: %+v %v", view, err)
	}
	if view.LiveErr == nil || !strings.Contains(view.LiveErr.Error(), "journal is being compacted") || len(view.Live) != 0 {
		t.Fatalf("the resume inspection's error is lost: %+v", view)
	}
}
