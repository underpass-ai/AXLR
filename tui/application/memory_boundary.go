package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session under an execution's policy is a bot's run, not a person's
// (docs/runtime-contract.md, "Policy per execution"): the program that
// runs it decided what it may do. Its memory is bound the same way (8
// November 2026, the QA of the bots' memory): the bot reads and writes
// only its own abouts, the person's about user:<login> neither reaches its
// prompt nor takes what it settles, and a new task wakes what the bot's
// previous task recorded, which a person's session only does when resumed.

// memoryBound reports whether the session's memory is bound to its own
// abouts: it runs under an execution's policy.
func memoryBound(s domain.Session) bool {
	_, has := s.Policy()
	return has
}

// boundAbouts are the abouts a bound session may read and write: the
// session's memory about (the selected one, by default the workspace's
// project about) and its own ws: about.
func boundAbouts(ctx context.Context, labels SessionLabelsPort, state domain.SessionState) ([]string, error) {
	about, _, err := memoryAbout(ctx, labels, state)
	if err != nil {
		return nil, err
	}
	own := sessionAbout(state.ID)
	if about == own {
		return []string{own}, nil
	}
	return []string{about, own}, nil
}

// checkBoundAbout refuses an about outside a bound session's own, with the
// ones it may name.
func checkBoundAbout(ctx context.Context, labels SessionLabelsPort, session domain.Session, about string) error {
	if about == "" || !memoryBound(session) {
		return nil
	}
	allowed, err := boundAbouts(ctx, labels, session.Export())
	if err != nil {
		return err
	}
	for _, own := range allowed {
		if about == own {
			return nil
		}
	}
	return fmt.Errorf("the execution's policy keeps memory to its own abouts (%s): %s is another's", strings.Join(allowed, ", "), about)
}

// errNoPreviousSession says a bound session has no earlier one to wake.
var errNoPreviousSession = errors.New("no previous session in the workspace")

// previousSession is the most recently saved session of the same workspace
// that has messages and is not this one: for a bot, its previous task (a
// node gives each bot one workspace). A person's session is not woken
// this way: the console wakes a session's own history when it is resumed.
func previousSession(ctx context.Context, store SessionStorePort, state domain.SessionState) (domain.SessionID, error) {
	if store == nil {
		return "", errNoPreviousSession
	}
	summaries, err := store.List(ctx)
	if err != nil {
		return "", err
	}
	var candidates []domain.SessionSummary
	for _, summary := range summaries {
		if summary.Workspace != state.Workspace || summary.ID == state.ID || summary.MessageCount == 0 {
			continue
		}
		candidates = append(candidates, summary)
	}
	if len(candidates) == 0 {
		return "", errNoPreviousSession
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt) })
	return candidates[0].ID, nil
}
