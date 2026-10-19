package storage_test

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// How much of real use axlr-v2 can judge whole: every person prompt of the
// sessions in AXLR_JEV_V2_SESSIONS (a copy of a session store; loading may
// migrate files) is compiled as the final check and the settlement gate
// would compile it, offline, and the states over MaxJudgementStateBytes
// are counted. Only counts and sizes are reported, never content.
func TestJudgeV2CoverageOfRecordedSessions(t *testing.T) {
	dir := os.Getenv("AXLR_JEV_V2_SESSIONS")
	if dir == "" {
		t.Skip("AXLR_JEV_V2_SESSIONS not set")
	}
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	type tally struct {
		asked, over, other int
		sizes              []int
	}
	families := map[string]*tally{application.JudgeFamilyFinal: {}, application.JudgeFamilyGate: {}}
	sessions := 0
	for _, summary := range summaries {
		s, err := store.Load(context.Background(), summary.ID)
		if err != nil {
			continue
		}
		sessions++
		messages := s.Messages()
		for i, message := range messages {
			if message.Role != root.RoleUser || strings.HasPrefix(string(message.Content), "[AXLR") {
				continue
			}
			for family, final := range map[string]bool{application.JudgeFamilyFinal: true, application.JudgeFamilyGate: false} {
				evidence, err := application.JudgeTurnEvidenceOf(messages, i, final)
				if err != nil {
					continue // not a turn this decision reads
				}
				if !final && len(evidence.Tools) == 0 {
					continue // settle asks only after results
				}
				count := families[family]
				count.asked++
				compiled, err := application.CompileJudgeV2(family, evidence.Dossier())
				switch {
				case errors.Is(err, application.ErrJudgeEvidenceTooLarge):
					count.over++
				case err != nil:
					count.other++
				default:
					count.sizes = append(count.sizes, len(compiled.Question.State))
				}
			}
		}
	}
	for family, count := range families {
		sort.Ints(count.sizes)
		p := func(q float64) int {
			if len(count.sizes) == 0 {
				return 0
			}
			return count.sizes[int(q*float64(len(count.sizes)-1))]
		}
		t.Logf("%s: %d sessions, %d decisions, %d over %d bytes (%.1f%%), %d other errors; state bytes p50 %d p90 %d max %d", family, sessions, count.asked, count.over, application.MaxJudgementStateBytes, 100*float64(count.over)/float64(max(1, count.asked)), count.other, p(0.5), p(0.9), p(1))
	}
}
