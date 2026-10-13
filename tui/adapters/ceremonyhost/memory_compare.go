package ceremonyhost

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
)

// compareBytes is the kmp_relate budget: the console compacts the packet.
const compareBytes = 9000

var _ application.MemoryComparePort = Memory{}

// Compare reads kmp_relate between the session's about and the others
// (decision D7 of the lifecycle integration design): what is declared
// between them, what KMP proposes from shared coordinates, and how many
// entries it read. It writes nothing; a proposal is a reading.
func (m Memory) Compare(ctx context.Context, request application.CompareRequest) (application.CompareResult, error) {
	about := strings.TrimSpace(request.About)
	if about == "" {
		return application.CompareResult{}, errors.New("compare needs the session's about")
	}
	abouts := []string{about}
	for _, other := range request.Others {
		if other = strings.TrimSpace(other); other != "" && !slices.Contains(abouts, other) {
			abouts = append(abouts, other)
		}
	}
	if len(abouts) < 2 {
		return application.CompareResult{}, errors.New("/memory compare needs at least one other about")
	}
	arguments := map[string]any{"about": about, "dimensions": map[string]any{"scope": "abouts", "abouts": abouts}, "budget": map[string]any{"max_bytes": compareBytes}}
	if request.ContextID != "" {
		arguments["context_id"] = request.ContextID
	}
	result, err := callPlugin(ctx, m.Tools, "kmp", "kmp_relate", arguments)
	if err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			return application.CompareResult{}, fmt.Errorf("KMP refused the comparison (%s): %s", refused.Code, bounded(refused.Message, 400))
		}
		return application.CompareResult{}, err
	}
	out := application.CompareResult{Abouts: abouts, Facts: map[string]int{}, Declared: relationsOf(result["declared"]), Proposals: relationsOf(result["coordinate"])}
	facts, _ := result["facts"].([]any)
	for _, raw := range facts {
		fact, _ := raw.(map[string]any)
		if name, _ := fact["about"].(string); name != "" {
			out.Facts[name]++
		}
	}
	return out, nil
}

// relationsOf reads kmp_relate's declared relations and coordinate
// proposals, which name their kind as rel or kind.
func relationsOf(raw any) []application.MemoryRelation {
	items, _ := raw.([]any)
	var out []application.MemoryRelation
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rel, _ := entry["rel"].(string)
		if rel == "" {
			rel, _ = entry["kind"].(string)
		}
		from, _ := entry["from"].(string)
		to, _ := entry["to"].(string)
		why, _ := entry["why"].(string)
		dimension, _ := entry["dimension"].(string)
		scope, _ := entry["scope_id"].(string)
		if from == "" || to == "" {
			continue
		}
		out = append(out, application.MemoryRelation{From: from, To: to, Rel: rel, Why: bounded(why, 300), Dimension: dimension, Scope: scope})
	}
	return out
}
