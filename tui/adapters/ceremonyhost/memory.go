package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
)

const wakeBytes = 2048

// Memory is application.MemoryPort over the connected KMP plugin.
type Memory struct{ Tools application.ToolExecutionPort }

var _ application.MemoryPort = Memory{}

func (m Memory) Wake(ctx context.Context, about string) (string, error) {
	packet, err := callPlugin(ctx, m.Tools, "kmp", "kmp_wake", map[string]any{"about": about, "budget": map[string]any{"max_bytes": wakeBytes}})
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) && (refused.Code == "not_found" || strings.Contains(refused.Message, "not found")) {
			return "", nil // a new session's about has no memory yet
		}
		return "", err
	}
	encoded, err := json.Marshal(packet)
	return string(encoded), err
}

func (m Memory) Record(ctx context.Context, about string, labels map[string][]string, id, summary, evidence string) error {
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", map[string]any{"about": about, "actor": "axlr", "labels": labels, "memories": []any{map[string]any{"id": id, "kind": "decision", "summary": summary, "evidence": evidence}}})
	if err != nil {
		return err
	}
	if accepted, _ := result["accepted"].(bool); accepted {
		return nil
	}
	if continuation, ok := result["continuation"].(string); ok && continuation != "" {
		_, err = callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", map[string]any{"continuation": continuation})
		return err
	}
	return errors.New("KMP did not accept the ceremony outcome")
}
