package application

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ErrNoSessionToContinue refuses axlr-tui -c in a workspace without a
// session to resume.
var ErrNoSessionToContinue = errors.New("no session to continue in this workspace")

// LatestSessionUseCase finds the session `axlr-tui -c` resumes: the most
// recently saved one of the workspace that has messages, is not archived
// and is not a plan task's worker. On 12 October 2026 --session needed the
// 32-hex ID, which only the exit hint showed.
type LatestSessionUseCase struct {
	Store SessionStorePort
	// Labels hides archived sessions; nil hides none.
	Labels SessionLabelsPort
}

func (u LatestSessionUseCase) Execute(ctx context.Context, workspace domain.Workspace) (domain.SessionID, error) {
	if u.Store == nil {
		return "", errors.New("latest session requires a store")
	}
	summaries, err := u.Store.List(ctx)
	if err != nil {
		return "", err
	}
	labels := map[domain.SessionID]domain.SessionLabel{}
	if u.Labels != nil {
		if labels, err = u.Labels.Load(ctx); err != nil {
			return "", err
		}
	}
	var candidates []domain.SessionSummary
	for _, s := range summaries {
		if s.Workspace == workspace && s.MessageCount > 0 && s.Mode != domain.ModeTask && !labels[s.ID].Archived {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("%w %s", ErrNoSessionToContinue, workspace)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt) })
	return candidates[0].ID, nil
}
