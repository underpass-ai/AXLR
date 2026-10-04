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
	text, _, err := m.WakeFocused(ctx, about, "")
	return text, err
}

// WakeFocused passes the intent to KMP, which (with Jev configured) keeps the
// evidence it judges relevant to it; the refs let the model link new memories
// to what was actually shown.
func (m Memory) WakeFocused(ctx context.Context, about, intent string) (string, []string, error) {
	arguments := map[string]any{"about": about, "budget": map[string]any{"max_bytes": wakeBytes}}
	if intent = strings.TrimSpace(intent); intent != "" {
		arguments["intent"] = bounded(intent, 600)
	}
	packet, err := callPlugin(ctx, m.Tools, "kmp", "kmp_wake", arguments)
	if err != nil {
		var refused *refusal
		if strings.HasPrefix(about, "ws:") && errors.As(err, &refused) && (refused.Code == "not_found" || strings.Contains(refused.Message, "not found")) {
			return "", nil, nil // a new session's about has no memory yet
		}
		return "", nil, err
	}
	return wakeProse(packet), wakeRefs(packet), nil
}

// wakeRefs lists the entry refs the packet exposed in its state sections.
func wakeRefs(packet map[string]any) []string {
	wake, _ := packet["wake"].(map[string]any)
	var refs []string
	seen := map[string]bool{}
	for _, section := range []string{"current_state", "open_loops", "next_actions"} {
		items, _ := wake[section].([]any)
		for _, item := range items {
			text, _ := item.(string)
			ref, _, _ := strings.Cut(text, " ")
			if strings.Contains(ref, ":entry:") && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

func bounded(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && text[cut]&0xC0 == 0x80 {
		cut--
	}
	return text[:cut]
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
	_, err := m.RecordLinked(ctx, about, labels, application.MemoryRecord{ID: id, Kind: "observation", Summary: summary, Evidence: evidence})
	return err
}

// RecordLinked writes one memory; each link carries the model's why and the
// console's evidence. KMP validates the stored endpoint of every ref.
func (m Memory) RecordLinked(ctx context.Context, about string, labels map[string][]string, record application.MemoryRecord) (string, error) {
	// summary_en is deliberately not sent: strict KMP refuses an English
	// summary that drops an identifier the summary carries (seen 5 Oct 2026),
	// and the console cannot promise that for model-written text.
	memory := map[string]any{"id": record.ID, "kind": record.Kind, "summary": record.Summary, "evidence": record.Evidence}
	if len(record.Links) > 0 {
		var links []any
		for _, link := range record.Links {
			entry := map[string]any{"ref": link.Ref, "rel": link.Rel, "why": link.Why, "confidence": "medium"}
			if link.Evidence != "" {
				entry["evidence"] = link.Evidence
			}
			links = append(links, entry)
		}
		memory["connect_to"] = links
	}
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", map[string]any{"about": about, "actor": "axlr", "idempotency_key": "axlr:ceremony-outcome:" + record.ID, "labels": labels, "memories": []any{memory}})
	if err != nil {
		return "", err
	}
	if accepted, _ := result["accepted"].(bool); accepted {
		refs, _ := result["local_refs"].(map[string]any)
		ref, _ := refs[record.ID].(string)
		return ref, nil
	}
	if status, _ := result["status"].(string); status == "needs_review" {
		return "", errors.New("KMP outcome needs writer review; inspect the returned context before resuming")
	}
	return "", errors.New("KMP did not accept the ceremony outcome")
}
