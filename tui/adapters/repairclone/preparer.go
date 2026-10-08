// Package repairclone prepares the disposable clone a repair or an
// improvement works in: the launcher's --repair and --improve and the agent's
// axlr_request_repair all use it.
package repairclone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Preparer is application.RepairClonePort over gh and git, run with the
// console's restricted environment.
type Preparer struct {
	// Repairs is the directory the clones live in.
	Repairs string
	// Env is the restricted environment the console gives local processes;
	// programs are resolved on its PATH, not the launcher's.
	Env    []string
	Stderr io.Writer
	Now    func() time.Time
}

var _ application.RepairClonePort = Preparer{}

func (p Preparer) run(ctx context.Context, dir, program string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir, cmd.Env = dir, p.Env
	if path, err := LookPath(p.Env, program); err == nil {
		cmd.Path = path
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", program, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Issue reads a GitHub issue as a brief: title, body and URL.
func (p Preparer) Issue(ctx context.Context, repository, number string) (string, error) {
	out, err := p.run(ctx, "", "gh", "issue", "view", number, "--repo", repository, "--json", "title,body,url", "--jq", `"\(.title)\n\n\(.body)\n\n\(.url)"`)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Issue #%s of %s: %s", number, repository, strings.TrimSpace(out)), nil
}

// Prepare clones the repository into <Repairs>/<slug>, writes the marker the
// repair ceremony requires and keeps the marker out of the repair commit.
func (p Preparer) Prepare(ctx context.Context, request application.RepairCloneRequest) (application.RepairClone, error) {
	switch {
	case request.Repository == "":
		return application.RepairClone{}, errors.New("a repair needs a repository")
	case request.Slug == "" || strings.ContainsAny(request.Slug, `/\`) || strings.HasPrefix(request.Slug, "."):
		return application.RepairClone{}, errors.New("a repair needs a plain slug")
	case p.Repairs == "" || !filepath.IsAbs(p.Repairs):
		return application.RepairClone{}, errors.New("the repairs directory must be absolute")
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	stderr := p.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	clone := filepath.Join(p.Repairs, request.Slug)
	if err := os.MkdirAll(p.Repairs, 0o700); err != nil {
		return application.RepairClone{}, err
	}
	if _, err := os.Lstat(clone); err == nil {
		return application.RepairClone{}, fmt.Errorf("%s already exists", clone)
	}
	fmt.Fprintln(stderr, "axlr-tui: cloning", request.Repository, "into", clone)
	if _, err := p.run(ctx, "", "gh", "repo", "clone", request.Repository, clone, "--", "--quiet"); err != nil {
		return application.RepairClone{}, err
	}
	base, err := p.run(ctx, clone, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return application.RepairClone{}, err
	}
	about := request.About
	if about == "" {
		about = "project:" + strings.ToLower(request.Repository[strings.LastIndex(request.Repository, "/")+1:])
	}
	brief := request.Brief
	if len(brief) > 8<<10 {
		brief = brief[:8<<10]
	}
	marker := application.RepairMarkerFile{Version: 1, Repository: request.Repository, Base: strings.TrimSpace(base), Slug: request.Slug, Brief: brief, About: about, Issue: request.Issue, Kind: request.Kind, Created: now().UTC().Format(time.RFC3339), Origin: string(request.Origin), Build: request.Build}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return application.RepairClone{}, err
	}
	if err := os.WriteFile(filepath.Join(clone, application.RepairMarker), append(data, '\n'), 0o600); err != nil {
		return application.RepairClone{}, err
	}
	// The marker is the clone's, not the repository's: keep it out of the
	// repair commit without touching the repository's own ignore file.
	exclude := filepath.Join(clone, ".git", "info", "exclude")
	if file, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_, _ = fmt.Fprintln(file, application.RepairMarker)
		_ = file.Close()
	}
	return application.RepairClone{Path: clone, Base: marker.Base}, nil
}

// LookPath finds program on the PATH entry of env.
func LookPath(env []string, program string) (string, error) {
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			for _, dir := range filepath.SplitList(value) {
				candidate := filepath.Join(dir, program)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
					return candidate, nil
				}
			}
		}
	}
	return "", exec.ErrNotFound
}
