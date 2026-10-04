package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
)

// wakeBytes is the packet budget. Below it KMP shortens the core prose to
// "…" (seen with 2 KiB on 2 Oct 2026); the driver keeps only the prose.
const wakeBytes = 12000

// Memory is application.MemoryPort over the connected KMP plugin.
type Memory struct{ Tools application.ToolExecutionPort }

var _ application.MemoryPort = Memory{}

func (m Memory) Wake(ctx context.Context, about string) (string, error) {
	packet, err := callPlugin(ctx, m.Tools, "kmp", "kmp_wake", map[string]any{"about": about, "budget": map[string]any{"max_bytes": wakeBytes}})
	if err != nil {
		var refused *refusal
		if strings.HasPrefix(about, "ws:") && errors.As(err, &refused) && (refused.Code == "not_found" || strings.Contains(refused.Message, "not found")) {
			return "", nil // a new session's about has no memory yet
		}
		return "", err
	}
	return wakeProse(packet), nil
}

// wakeProse retains reference-bearing state and exposes unfinished recall.
func wakeProse(packet map[string]any) string {
	wake, _ := packet["wake"].(map[string]any)
	var lines []string
	projection, _ := packet["projection"].(map[string]any)
	page, _ := projection["page"].(map[string]any)
	hasMore, _ := page["has_more"].(bool)
	shortened, _ := projection["core_text_shortened"].(bool)
	if hasMore || shortened {
		action, _ := json.Marshal(projection["next_action"])
		lines = append(lines, "Partial KMP recall; do not treat it as complete. Resume action: "+string(action))
	}
	for _, section := range []string{"current_state", "open_loops", "next_actions"} {
		items, _ := wake[section].([]any)
		for _, item := range items {
			text, _ := item.(string)
			if text = strings.TrimSpace(text); text != "" && text != "…" {
				lines = append(lines, "- "+text)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m Memory) Record(ctx context.Context, about string, labels map[string][]string, id, summary, evidence string) error {
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", map[string]any{"about": about, "actor": "axlr", "idempotency_key": "axlr:ceremony-outcome:" + id, "labels": labels, "memories": []any{map[string]any{"id": id, "kind": "observation", "summary": summary, "evidence": evidence}}})
	if err != nil {
		return err
	}
	if accepted, _ := result["accepted"].(bool); accepted {
		return nil
	}
	if status, _ := result["status"].(string); status == "needs_review" {
		return errors.New("KMP outcome needs writer review; inspect the returned context before resuming")
	}
	return errors.New("KMP did not accept the ceremony outcome")
}
