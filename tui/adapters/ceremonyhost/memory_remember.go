package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
)

var _ application.RememberPort = Memory{}

// rememberContextItems and rememberTextBytes bound the stored context a
// review returns to the model: KMP's own review of one linked memory was
// 2.9 KB (9 October 2026), most of it actions the model does not need.
const (
	rememberContextItems = 8
	rememberTextBytes    = 300
)

// Remember writes axlr_remember's memory with kmp_write_memory, as the
// work identity, and answers compactly: the stored ref, or KMP's review of
// the links with the continuation that commits it. KMP needs no guide for
// a write, so an empty store accepts it.
func (m Memory) Remember(ctx context.Context, request application.RememberRequest) (map[string]any, error) {
	arguments := map[string]any{"continuation": request.Continuation}
	if request.Continuation == "" {
		memory := map[string]any{"id": "m1", "kind": request.Kind, "summary": request.Text, "evidence": strings.Join(request.Evidence, "\n")}
		var links []any
		for _, link := range request.Links {
			evidence := link.Evidence
			if evidence == "" {
				evidence = request.Evidence[0]
			}
			links = append(links, map[string]any{"ref": link.Ref, "rel": link.Rel, "why": link.Why, "evidence": evidence, "confidence": "medium"})
		}
		if len(links) > 0 {
			memory["connect_to"] = links
		}
		arguments = map[string]any{"about": request.About, "actor": "axlr", "idempotency_key": request.IdempotencyKey, "labels": request.Labels, "memories": []any{memory}}
	}
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_write_memory", arguments)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			return nil, fmt.Errorf("KMP refused the memory (%s): %s", refused.Code, bounded(refused.Message, 1000))
		}
		return nil, err
	}
	status, _ := result["status"].(string)
	if accepted, _ := result["accepted"].(bool); accepted {
		refs, _ := result["local_refs"].(map[string]any)
		ref, _ := refs["m1"].(string)
		about, key := request.About, request.IdempotencyKey
		if about == "" || key == "" {
			// A committed continuation names them in its receipt only.
			about, key = receiptScope(result)
		}
		return map[string]any{"accepted": true, "status": status, "ref": ref, "about": about, "idempotency_key": key}, nil
	}
	if status != "needs_review" {
		summary, _ := result["summary"].(string)
		return nil, fmt.Errorf("KMP did not write the memory (status %q): %s", status, bounded(summary, 600))
	}
	return reviewAnswer(result, request), nil
}

// reviewAnswer is KMP's needs_review, compact: the links it would write,
// the stored context they touch and the continuation that commits them.
func reviewAnswer(result map[string]any, request application.RememberRequest) map[string]any {
	answer := map[string]any{"accepted": false, "needs_review": true, "about": request.About,
		"instruction": "Nothing was written. Check that each link joins the right memories in the right direction against the stored context. If it does, call axlr_remember with only {\"continuation\": ...} to commit; otherwise call it again with corrected links."}
	if relations, ok := result["relations"].([]any); ok {
		answer["links"] = relations
	}
	if neighborhood, ok := result["neighborhood"].(map[string]any); ok {
		items, _ := neighborhood["items"].([]any)
		var context []any
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok || len(context) == rememberContextItems {
				continue
			}
			text, _ := item["text"].(string)
			context = append(context, map[string]any{"ref": item["ref"], "kind": item["kind"], "state": item["state"], "text": bounded(text, rememberTextBytes)})
		}
		answer["context"] = context
		if len(items) > rememberContextItems {
			answer["context_omitted"] = len(items) - rememberContextItems
		}
	}
	if actions, ok := result["next_actions"].([]any); ok {
		for _, raw := range actions {
			action, _ := raw.(map[string]any)
			arguments, _ := action["arguments"].(map[string]any)
			if continuation, ok := arguments["continuation"].(string); ok && action["tool"] == "kmp_write_memory" {
				answer["continuation"] = continuation
				break
			}
		}
	}
	return answer
}

// receiptScope reads the about and idempotency key of a write from its
// receipt ref, receipt:v1:<about>:<key> with both escaped.
func receiptScope(result map[string]any) (about, key string) {
	receipt, _ := result["receipt"].(map[string]any)
	ref, _ := receipt["ref"].(string)
	parts := strings.Split(ref, ":")
	if len(parts) != 4 || parts[0] != "receipt" {
		return "", ""
	}
	about, aboutErr := url.PathUnescape(parts[2])
	key, keyErr := url.PathUnescape(parts[3])
	if aboutErr != nil || keyErr != nil {
		return "", ""
	}
	return about, key
}
