package application

import "context"

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
}
