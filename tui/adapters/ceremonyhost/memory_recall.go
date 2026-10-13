package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Recall budgets: KMP shortens its core text below the budget (a 3 000-byte
// kmp_ask elided the answer to "…" on 13 October 2026), so the console asks
// for a generous packet and compacts it itself.
const (
	recallWakeBytes    = 12000
	recallAskBytes     = 9000
	recallHistoryBytes = 8000
	recallEvidenceMax  = 8
	recallTextBytes    = 600
)

var _ application.RecallPort = Memory{}

// Recall reads memory for the console or for axlr_recall: a wake, a
// question or a span of history under one about, in the session's context.
// An about nothing was recorded under is Empty, not an error, for every
// about: the console's recall note and the model's question both ask about
// abouts that may not exist yet (a new project, a person's user about).
func (m Memory) Recall(ctx context.Context, request application.RecallRequest) (application.RecallResult, error) {
	about := strings.TrimSpace(request.About)
	if about == "" {
		return application.RecallResult{}, errors.New("recall needs an about")
	}
	result := application.RecallResult{About: about, Mode: request.Mode}
	var tool string
	arguments := map[string]any{"about": about}
	if request.ContextID != "" {
		arguments["context_id"] = string(request.ContextID)
	}
	switch request.Mode {
	case "", application.RecallWake:
		tool, result.Mode = "kmp_wake", application.RecallWake
		arguments["budget"] = map[string]any{"max_bytes": recallWakeBytes}
		if intent := strings.TrimSpace(request.Intent); intent != "" {
			arguments["intent"] = bounded(intent, 600)
		}
		if request.Since != "" {
			arguments["interval"] = map[string]any{"start": request.Since}
		}
	case application.RecallAsk:
		tool = "kmp_ask"
		question := strings.TrimSpace(request.Intent)
		if question == "" {
			return application.RecallResult{}, errors.New("a question is required to ask memory")
		}
		arguments["question"] = bounded(question, 2000)
		arguments["budget"] = map[string]any{"max_bytes": recallAskBytes}
		if request.Since != "" {
			arguments["interval"] = map[string]any{"start": request.Since}
		}
	case application.RecallHistory:
		tool = "kmp_time"
		entries := request.Entries
		if entries <= 0 {
			entries = 10
		}
		arguments["limit"] = map[string]any{"entries": entries}
		arguments["budget"] = map[string]any{"max_bytes": recallHistoryBytes}
		if request.Since != "" {
			// Oldest first from the instant; KMP needs from or interval
			// (seen on 13 October 2026: "move rewind requires `from`").
			arguments["move"], arguments["interval"] = "forward", map[string]any{"start": request.Since}
		} else {
			now := time.Now
			if m.Now != nil {
				now = m.Now
			}
			arguments["move"], arguments["from"] = "rewind", map[string]any{"time": now().UTC().Format(time.RFC3339)}
		}
	default:
		return application.RecallResult{}, fmt.Errorf("unknown recall mode %q", request.Mode)
	}
	packet, err := callPlugin(ctx, m.Tools, "kmp", tool, arguments)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) && (refused.Code == "not_found" || strings.Contains(refused.Message, "not found")) {
			result.Empty = true
			return result, nil
		}
		if errors.As(err, &refused) {
			return application.RecallResult{}, fmt.Errorf("KMP refused the %s (%s): %s", tool, refused.Code, bounded(refused.Message, 600))
		}
		return application.RecallResult{}, err
	}
	switch result.Mode {
	case application.RecallWake:
		result.Text, result.Refs = wakeProse(packet), wakeRefs(packet)
		result.Empty = result.Text == ""
	case application.RecallAsk:
		result.Answer = askAnswer(packet)
	case application.RecallHistory:
		result.Answer = historyAnswer(packet)
	}
	return result, nil
}

// askAnswer compacts a kmp_ask packet: the answer and its status, the
// evidence KMP retrieved (text bounded, with its ref and time), its
// warnings and whether more exists. Without Jev the answer is a pointer at
// the evidence, so the evidence is the payload; UNKNOWN passes as it is.
func askAnswer(packet map[string]any) map[string]any {
	answer := map[string]any{}
	for _, key := range []string{"answer", "answer_status", "summary"} {
		if value, ok := packet[key].(string); ok && value != "" {
			answer[key] = value
		}
	}
	proof, _ := packet["proof"].(map[string]any)
	if confidence, ok := proof["confidence"].(string); ok && confidence != "" {
		answer["confidence"] = confidence
	}
	var evidence []map[string]any
	items, _ := proof["evidence"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		text, _ := entry["text"].(string)
		if text == "" {
			continue
		}
		compact := map[string]any{"text": bounded(text, recallTextBytes)}
		if supports, _ := entry["supports"].([]any); len(supports) > 0 {
			compact["ref"] = supports[0]
		} else if id, _ := entry["id"].(string); id != "" {
			compact["ref"] = id
		}
		if at, _ := entry["time"].(string); at != "" {
			compact["time"] = at
		}
		evidence = append(evidence, compact)
		if len(evidence) == recallEvidenceMax {
			break
		}
	}
	if len(evidence) > 0 {
		answer["evidence"] = evidence
	}
	if nearest, ok := proof["nearest_outside"]; ok && nearest != nil {
		answer["nearest_outside"] = nearest
	}
	if warnings, _ := packet["warnings"].([]any); len(warnings) > 0 {
		answer["warnings"] = warnings
	}
	if projection, _ := packet["projection"].(map[string]any); projection != nil {
		page, _ := projection["page"].(map[string]any)
		if more, _ := page["has_more"].(bool); more {
			answer["partial"] = true
		}
	}
	return answer
}

// historyAnswer compacts a kmp_time packet: the entries in order (ref,
// kind, text, when) and whether more exist.
func historyAnswer(packet map[string]any) map[string]any {
	answer := map[string]any{}
	if summary, ok := packet["summary"].(string); ok && summary != "" {
		answer["summary"] = summary
	}
	var entries []map[string]any
	items, _ := packet["entries"].([]any)
	for _, item := range items {
		entry, _ := item.(map[string]any)
		compact := map[string]any{}
		for _, key := range []string{"ref", "kind"} {
			if value, ok := entry[key].(string); ok && value != "" {
				compact[key] = value
			}
		}
		if text, _ := entry["text"].(string); text != "" {
			compact["text"] = bounded(text, recallTextBytes)
		}
		if coordinates, _ := entry["coordinates"].([]any); len(coordinates) > 0 {
			first, _ := coordinates[0].(map[string]any)
			if at, _ := first["occurred_at"].(string); at != "" {
				compact["occurred_at"] = at
			}
		}
		if len(compact) > 0 {
			entries = append(entries, compact)
		}
	}
	answer["entries"] = entries
	if entries == nil {
		answer["entries"] = []map[string]any{}
	}
	if page, _ := packet["page"].(map[string]any); page != nil {
		if more, _ := page["has_more"].(bool); more {
			answer["partial"] = true
		}
	}
	if warnings, _ := packet["warnings"].([]any); len(warnings) > 0 {
		answer["warnings"] = warnings
	}
	return answer
}
