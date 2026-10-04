package main

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
	"unicode"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// prepareRepairClone clones the configured repository into a fresh directory
// under repairs (or the configured directory) and writes the marker that lets
// /repair start there. "#123" reads the issue as the brief. It returns the
// clone path and the brief that fills the composer.
func prepareRepairClone(ctx context.Context, settings storage.RepairSettings, repairs, request string, env []string, stderr io.Writer) (string, string, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return "", "", errors.New("--repair needs a failure brief or #issue")
	}
	if settings.Directory != "" {
		repairs = settings.Directory
	}
	run := func(dir, program string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Dir, cmd.Env = dir, env
		// Resolve the program on the restricted PATH, as the console's exec
		// does, not on the launcher's own environment.
		if path, err := lookPath(env, program); err == nil {
			cmd.Path = path
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%s %s: %w: %s", program, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}
	brief, issue := request, ""
	if strings.HasPrefix(request, "#") && len(request) > 1 && strings.IndexFunc(request[1:], func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
		issue = request[1:]
		out, err := run("", "gh", "issue", "view", issue, "--repo", settings.Repository, "--json", "title,body,url", "--jq", `"\(.title)\n\n\(.body)\n\n\(.url)"`)
		if err != nil {
			return "", "", err
		}
		brief = fmt.Sprintf("Issue #%s of %s: %s", issue, settings.Repository, strings.TrimSpace(out))
	}
	if len(brief) > 8<<10 {
		brief = brief[:8<<10]
	}
	now := time.Now().UTC()
	slug := repairSlug(brief, now)
	clone := filepath.Join(repairs, slug)
	if err := os.MkdirAll(repairs, 0o700); err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(clone); err == nil {
		return "", "", fmt.Errorf("%s already exists", clone)
	}
	fmt.Fprintln(stderr, "axlr-tui: cloning", settings.Repository, "into", clone)
	if _, err := run("", "gh", "repo", "clone", settings.Repository, clone, "--", "--quiet"); err != nil {
		return "", "", err
	}
	base, err := run(clone, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	about := settings.About
	if about == "" {
		about = "project:" + strings.ToLower(settings.Repository[strings.LastIndex(settings.Repository, "/")+1:])
	}
	marker := application.RepairMarkerFile{Version: 1, Repository: settings.Repository, Base: strings.TrimSpace(base), Slug: slug, Brief: brief, About: about, Issue: issue, Created: now.Format(time.RFC3339)}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(clone, application.RepairMarker), append(data, '\n'), 0o600); err != nil {
		return "", "", err
	}
	// The marker is the clone's, not the repository's: keep it out of the
	// repair commit without touching the repository's own ignore file.
	exclude := filepath.Join(clone, ".git", "info", "exclude")
	if file, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_, _ = fmt.Fprintln(file, application.RepairMarker)
		_ = file.Close()
	}
	return clone, brief, nil
}

// lookPath finds program on the PATH entry of env.
func lookPath(env []string, program string) (string, error) {
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

// repairSlug names the clone directory and the branch: a timestamp and the
// brief's first words, kebab-cased.
func repairSlug(brief string, now time.Time) string {
	var words []string
	length := 0
	for _, field := range strings.Fields(strings.ToLower(brief)) {
		var word strings.Builder
		for _, r := range field {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				word.WriteRune(r)
			}
		}
		if word.Len() == 0 {
			continue
		}
		if length+word.Len()+1 > 40 {
			break
		}
		words = append(words, word.String())
		length += word.Len() + 1
	}
	if len(words) == 0 {
		words = []string{"failure"}
	}
	return now.Format("20060102-1504") + "-" + strings.Join(words, "-")
}
