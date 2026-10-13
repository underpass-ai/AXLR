package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// wakeBytes is the packet budget. Below it KMP shortens the core prose to
// "…" (seen with 2 KiB on 2 Oct 2026); the driver keeps only the prose.
const wakeBytes = 12000

// Memory is application.MemoryPort over the connected KMP plugin.
type Memory struct {
	Tools application.ToolExecutionPort
	// SyncGuide writes the guide into the store when kmp_guide reports it
	// missing; nil leaves the refusal to the caller.
	SyncGuide func(context.Context) error
	// Now is the clock a history read rewinds from; nil is time.Now.
	Now func() time.Time
}

var _ application.MemoryPort = Memory{}
var _ application.MemoryIdentityPort = Memory{}

// guideMissing reports kmp_guide's refusal for a store without the guide
// (seen on 13 October 2026: "The selected store cannot serve guide node
// `guide:kmp-agent:overview`… Run `kmp-mcp guide sync --plugin-root`").
func guideMissing(err error) bool {
	var refused *refusal
	if !errors.As(err, &refused) {
		return false
	}
	text := strings.ToLower(refused.Message)
	return strings.Contains(text, "guide sync") || strings.Contains(text, "cannot serve guide node")
}

// Guide registers the console's agent (registrationKey) or opens a context
// for a known agent (agentID with contextKey). A store without the guide is
// synced once and the call repeated.
func (m Memory) Guide(ctx context.Context, registrationKey, agentID, contextKey string) (application.GuideResult, error) {
	arguments := map[string]any{}
	if registrationKey != "" {
		arguments["registration_key"] = registrationKey
	} else {
		arguments["agent_id"], arguments["context_key"] = agentID, contextKey
	}
	packet, err := callPlugin(ctx, m.Tools, "kmp", "kmp_guide", arguments)
	if err != nil && guideMissing(err) && m.SyncGuide != nil {
		if syncErr := m.SyncGuide(ctx); syncErr != nil {
			return application.GuideResult{}, fmt.Errorf("%w; %v", err, syncErr)
		}
		packet, err = callPlugin(ctx, m.Tools, "kmp", "kmp_guide", arguments)
	}
	if err != nil {
		return application.GuideResult{}, err
	}
	var result application.GuideResult
	agent, _ := packet["agent"].(map[string]any)
	result.AgentID, _ = agent["id"].(string)
	result.ContextID, _ = packet["context_id"].(string)
	if result.ContextID == "" || (registrationKey != "" && result.AgentID == "") {
		return application.GuideResult{}, errors.New("kmp_guide returned no agent or context")
	}
	return result, nil
}

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

var _ application.JournalPort = Memory{}

// RecordTurn writes a turn's results under the session's about as one
// turn entry: the console's English summary serves as summary_en too, so
// a strict store accepts it, and the evidence is one line per action.
func (m Memory) RecordTurn(ctx context.Context, entry application.TurnEntry) (string, error) {
	memory := map[string]any{"id": "turn", "kind": "turn", "summary": entry.Summary, "summary_en": entry.Summary, "evidence": strings.Join(entry.Evidence, "\n")}
	if !entry.OccurredAt.IsZero() {
		memory["occurred_at"] = entry.OccurredAt.UTC().Format(time.RFC3339)
	}
	arguments := map[string]any{"about": entry.About, "actor": "axlr", "idempotency_key": entry.Key, "labels": entry.Labels, "memories": []any{memory}}
	if entry.ContextID != "" {
		arguments["context_id"] = entry.ContextID
	}
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", arguments)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			return "", fmt.Errorf("KMP refused the turn entry (%s): %s", refused.Code, bounded(refused.Message, 600))
		}
		return "", err
	}
	if accepted, _ := result["accepted"].(bool); accepted {
		refs, _ := result["local_refs"].(map[string]any)
		ref, _ := refs["turn"].(string)
		return ref, nil
	}
	status, _ := result["status"].(string)
	return "", fmt.Errorf("KMP did not write the turn entry (status %q)", status)
}
