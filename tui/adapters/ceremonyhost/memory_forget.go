package ceremonyhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const (
	forgetInspectBytes = 3000
	forgetTextBytes    = 400
)

var _ application.MemoryForgetPort = Memory{}

// Forget closes one entry (decisions E15 and E16 of the lifecycle
// integration design): an observation under the entry's about that
// supersedes it, with the person's reason and command as evidence. The
// entry stays, superseded; nothing is removed. KMP answers needs_review
// for a link to a stored entry; the person decided, so the console
// commits the unchanged packet with the continuation.
func (m Memory) Forget(ctx context.Context, request application.ForgetRequest) (application.ForgetReceipt, error) {
	ref := strings.TrimSpace(request.Ref)
	about, err := application.RefAbout(ref)
	if err != nil {
		return application.ForgetReceipt{}, err
	}
	inspect := map[string]any{"about": about, "ref": ref, "budget": map[string]any{"max_bytes": forgetInspectBytes}, "include": map[string]any{"details": false, "incoming": false, "outgoing": false, "raw": false}}
	if request.ContextID != "" {
		inspect["context_id"] = string(request.ContextID)
	}
	found, err := callPlugin(ctx, m.Tools, "kmp", "kmp_inspect", inspect)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			return application.ForgetReceipt{}, fmt.Errorf("KMP does not hold %s (%s): %s", ref, refused.Code, bounded(refused.Message, 300))
		}
		return application.ForgetReceipt{}, err
	}
	object, _ := found["object"].(map[string]any)
	text, _ := object["text"].(string)
	if strings.TrimSpace(text) == "" {
		return application.ForgetReceipt{}, fmt.Errorf("%s is not a memory entry", ref)
	}
	metadata, _ := object["metadata"].(map[string]any)
	english, _ := metadata["summary_en"].(string)
	if strings.TrimSpace(english) == "" {
		english = text
	}
	why := strings.TrimSpace(request.Why)
	if why == "" {
		why = application.ForgetWhy
	}
	at := request.At
	if at.IsZero() {
		at = m.clock()
	}
	command := strings.TrimSpace(request.Command)
	if command == "" {
		command = "/memory forget " + ref
	}
	evidence := command + " on " + at.UTC().Format(time.RFC3339) + ": " + why
	memory := map[string]any{"id": "forget", "kind": "observation",
		"summary":    "Forgotten at the person's request: " + bounded(text, forgetTextBytes),
		"summary_en": "Forgotten at the person's request: " + bounded(english, forgetTextBytes),
		"evidence":   evidence,
		"connect_to": []any{map[string]any{"ref": ref, "rel": "supersedes", "why": why, "confidence": "high", "evidence": evidence}}}
	labels := request.Labels
	if len(labels) == 0 {
		labels = map[string][]string{"event": {"forget"}}
	}
	digest := sha256.Sum256([]byte(ref))
	arguments := map[string]any{"about": about, "actor": "axlr", "idempotency_key": fmt.Sprintf("axlr:forget:%s:%d", hex.EncodeToString(digest[:8]), at.UTC().Unix()), "labels": labels, "memories": []any{memory}}
	if request.ContextID != "" {
		arguments["context_id"] = string(request.ContextID)
	}
	receipt := application.ForgetReceipt{About: about, Ref: ref, Text: text}
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", arguments)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			return receipt, fmt.Errorf("KMP refused to close %s (%s): %s", ref, refused.Code, bounded(refused.Message, 600))
		}
		return receipt, err
	}
	receipt.Superseding = localRef(result, "forget")
	if accepted, _ := result["accepted"].(bool); accepted {
		return receipt, nil
	}
	if status, _ := result["status"].(string); status != "needs_review" {
		summary, _ := result["summary"].(string)
		return receipt, fmt.Errorf("KMP did not close %s (status %q): %s", ref, status, bounded(summary, 400))
	}
	continuation := writeContinuation(result)
	if continuation == "" {
		return receipt, fmt.Errorf("KMP asked for a review of the link to %s and offered no continuation", ref)
	}
	// The continuation goes alone: any other argument, the context
	// included, starts a different call (KMP 0.25.0, 13 October 2026).
	committed, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", map[string]any{"continuation": continuation})
	if err != nil {
		return receipt, fmt.Errorf("KMP did not commit the close of %s: %w", ref, err)
	}
	if accepted, _ := committed["accepted"].(bool); !accepted {
		summary, _ := committed["summary"].(string)
		return receipt, fmt.Errorf("KMP did not commit the close of %s: %s", ref, bounded(summary, 400))
	}
	if ref := localRef(committed, "forget"); ref != "" {
		receipt.Superseding = ref
	}
	return receipt, nil
}

// localRef is the stored ref a write gave a memory id.
func localRef(result map[string]any, id string) string {
	refs, _ := result["local_refs"].(map[string]any)
	ref, _ := refs[id].(string)
	return ref
}

// writeContinuation is the kmp_write_memory continuation a review offers.
func writeContinuation(result map[string]any) string {
	actions, _ := result["next_actions"].([]any)
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		arguments, _ := action["arguments"].(map[string]any)
		if continuation, ok := arguments["continuation"].(string); ok && action["tool"] == "kmp_write_memory" {
			return continuation
		}
	}
	return ""
}

func (m Memory) clock() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
