package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/underpass-ai/AXLR/buildinfo"
	"github.com/underpass-ai/AXLR/tui/adapters/repairclone"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// prepareRepairClone is the launcher's --repair: it clones the configured
// repository into a fresh directory under repairs (or the configured
// directory) and writes the marker that lets /repair start there. "#123"
// reads the issue as the brief. It returns the clone path and the brief that
// fills the composer.
func prepareRepairClone(ctx context.Context, settings storage.RepairSettings, repairs, request string, env []string, stderr io.Writer) (string, string, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return "", "", errors.New("--repair needs a failure brief or #issue")
	}
	if settings.Directory != "" {
		repairs = settings.Directory
	}
	preparer := repairclone.Preparer{Repairs: repairs, Env: env, Stderr: stderr}
	brief, issue := request, ""
	if strings.HasPrefix(request, "#") && len(request) > 1 && strings.IndexFunc(request[1:], func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
		var err error
		issue = request[1:]
		if brief, err = preparer.Issue(ctx, settings.Repository, issue); err != nil {
			return "", "", err
		}
	}
	if len(brief) > 8<<10 {
		brief = brief[:8<<10]
	}
	clone, err := preparer.Prepare(ctx, application.RepairCloneRequest{Repository: settings.Repository, Brief: brief, About: settings.About, Slug: application.RepairSlug(brief, time.Now().UTC()), Issue: issue, Build: buildinfo.Version})
	if err != nil {
		return "", "", err
	}
	return clone.Path, brief, nil
}
