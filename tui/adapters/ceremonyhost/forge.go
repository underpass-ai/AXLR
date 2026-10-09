package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// CommitAuthor and CommitEmail sign the console's repair commits.
const (
	CommitAuthor = "AXLR"
	CommitEmail  = "axlr@underpass.ai"
)

// Forge is application.ForgePort over git and the GitHub CLI, run through the
// console's exec runtime in the clone: the same restricted environment and
// workspace boundary as the model's commands, and no shell.
type Forge struct{ Checks application.CheckRunnerPort }

var _ application.ForgePort = Forge{}

// run returns the whole stdout of a successful command, for parsing: it asks
// the runtime for its whole output limit and refuses an output cut there.
// Failures quote only the tail of stdout and stderr, which is enough to show
// the model what went wrong.
func (f Forge) run(ctx context.Context, program string, args ...string) (string, error) {
	result, err := f.Checks.Run(ctx, domain.CheckCommand{Program: program, Args: args, MaxOutput: consoleOutput})
	if err != nil {
		return "", err
	}
	if !result.Ran || result.ExitCode != 0 {
		return result.Output, fmt.Errorf("%s %s: exit %d: %s", program, strings.Join(args, " "), result.ExitCode, strings.TrimSpace(result.Output))
	}
	if result.Truncated {
		return "", fmt.Errorf("%s %s: its output was cut at the runtime's %d-byte limit; the console does not parse a partial answer", program, strings.Join(args, " "), consoleOutput)
	}
	if result.Stdout != "" {
		return result.Stdout, nil
	}
	return result.Output, nil
}

func (f Forge) Propose(ctx context.Context, p application.RepairProposal) (application.PullRequest, error) {
	if p.Repository == "" || p.Branch == "" || p.Base == "" {
		return application.PullRequest{}, errors.New("a proposal needs repository, base and branch")
	}
	if p.Number == 0 {
		if _, err := f.run(ctx, "git", "checkout", "-B", p.Branch); err != nil {
			return application.PullRequest{}, err
		}
	}
	if _, err := f.run(ctx, "git", "add", "-A"); err != nil {
		return application.PullRequest{}, err
	}
	staged, _ := f.Checks.Run(ctx, domain.CheckCommand{Program: "git", Args: []string{"diff", "--cached", "--quiet"}})
	if staged.Ran && staged.ExitCode == 0 {
		return application.PullRequest{}, errors.New("the repair changed no file in the clone")
	}
	message := p.Title + "\n\n" + p.Body
	if p.Trailer != "" {
		message += "\n" + p.Trailer + "\n"
	}
	// The console is the author: a fresh clone has no identity of its own
	// and a person's name on a machine-made commit would mislead.
	if _, err := f.run(ctx, "git", "-c", "user.name="+CommitAuthor, "-c", "user.email="+CommitEmail, "-c", "commit.gpgsign=false", "commit", "-q", "-m", message); err != nil {
		return application.PullRequest{}, err
	}
	if _, err := f.run(ctx, "git", "push", "-q", "-u", "origin", p.Branch); err != nil {
		return application.PullRequest{}, err
	}
	head, err := f.run(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return application.PullRequest{}, err
	}
	pr := application.PullRequest{Number: p.Number, HeadSHA: strings.TrimSpace(head)}
	if p.Number == 0 {
		out, err := f.run(ctx, "gh", "pr", "create", "--repo", p.Repository, "--base", p.Base, "--head", p.Branch, "--title", p.Title, "--body", p.Body)
		if err != nil {
			return application.PullRequest{}, err
		}
		pr.URL = lastLine(out)
		number, err := strconv.Atoi(pr.URL[strings.LastIndex(pr.URL, "/")+1:])
		if err != nil || number <= 0 {
			return application.PullRequest{}, fmt.Errorf("gh pr create returned no pull request URL: %q", pr.URL)
		}
		pr.Number = number
		return pr, nil
	}
	out, err := f.run(ctx, "gh", "pr", "view", strconv.Itoa(p.Number), "--repo", p.Repository, "--json", "url", "--jq", ".url")
	if err != nil {
		return application.PullRequest{}, err
	}
	pr.URL = lastLine(out)
	return pr, nil
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

type pullRequestView struct {
	State             string `json:"state"`
	MergeStateStatus  string `json:"mergeStateStatus"`
	HeadRefOid        string `json:"headRefOid"`
	StatusCheckRollup []struct {
		Name       string `json:"name"`
		Context    string `json:"context"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
		DetailsURL string `json:"detailsUrl"`
		TargetURL  string `json:"targetUrl"`
	} `json:"statusCheckRollup"`
}

func (f Forge) Status(ctx context.Context, repository string, number int) (application.PullRequestStatus, error) {
	out, err := f.run(ctx, "gh", "pr", "view", strconv.Itoa(number), "--repo", repository, "--json", "state,mergeStateStatus,headRefOid,statusCheckRollup")
	if err != nil {
		return application.PullRequestStatus{}, err
	}
	var view pullRequestView
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &view); err != nil {
		return application.PullRequestStatus{}, fmt.Errorf("unreadable pull request view: %w", err)
	}
	status := application.PullRequestStatus{State: view.State, MergeState: view.MergeStateStatus, HeadSHA: view.HeadRefOid}
	for _, check := range view.StatusCheckRollup {
		name := check.Name
		if name == "" {
			name = check.Context
		}
		// Check runs carry status/conclusion; commit statuses carry state.
		conclusion := strings.ToUpper(check.Conclusion)
		if check.Status != "" && !strings.EqualFold(check.Status, "COMPLETED") {
			status.Pending++
			continue
		}
		if conclusion == "" {
			conclusion = strings.ToUpper(check.State)
		}
		switch conclusion {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			status.Passed++
		case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "WAITING", "":
			status.Pending++
		default:
			url := check.DetailsURL
			if url == "" {
				url = check.TargetURL
			}
			status.Failed = append(status.Failed, fmt.Sprintf("%s: %s %s", name, strings.ToLower(conclusion), url))
		}
	}
	return status, nil
}

func (f Forge) UpdateBranch(ctx context.Context, repository string, number int) error {
	_, err := f.run(ctx, "gh", "pr", "update-branch", strconv.Itoa(number), "--repo", repository)
	return err
}

func (f Forge) Merge(ctx context.Context, repository string, number int) (string, error) {
	if _, err := f.run(ctx, "gh", "pr", "merge", strconv.Itoa(number), "--repo", repository, "--squash", "--delete-branch"); err != nil {
		return "", err
	}
	out, err := f.run(ctx, "gh", "pr", "view", strconv.Itoa(number), "--repo", repository, "--json", "mergeCommit", "--jq", ".mergeCommit.oid")
	if err != nil {
		return "", err
	}
	return lastLine(out), nil
}
