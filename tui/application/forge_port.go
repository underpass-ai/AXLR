package application

import (
	"context"
	"errors"
)

// ForgePort is the Git hosting side of the repair ceremony as the driver sees
// it: branches, pull requests, checks and merges, never the model.
type ForgePort interface {
	// Propose commits the clone's changes on the branch, pushes it and opens
	// the pull request; with Number set it pushes to the open one instead.
	Propose(ctx context.Context, proposal RepairProposal) (PullRequest, error)
	// Status reads the pull request's checks and merge state.
	Status(ctx context.Context, repository string, number int) (PullRequestStatus, error)
	// UpdateBranch merges the base into the pull request branch on the forge.
	UpdateBranch(ctx context.Context, repository string, number int) error
	// Merge squash-merges the pull request, deletes its branch and returns
	// the merge commit.
	Merge(ctx context.Context, repository string, number int) (string, error)
	// RerunFailed reruns the failed checks of the pull request's head and
	// returns how many workflow runs it started again; a check that no
	// workflow run produced cannot be rerun and is skipped.
	RerunFailed(ctx context.Context, repository string, number int) (int, error)
}

// ErrNothingToPropose is Propose's answer when a later round of an open pull
// request changed no file: the model found nothing to fix in what the red
// checks showed, so the console reruns them instead of ending the ceremony.
var ErrNothingToPropose = errors.New("the repair changed no file in the clone")
