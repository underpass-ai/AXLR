package domain

import (
	"errors"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

const (
	MaxChangePreviewBytes = 64 << 10
	MaxChangePreviewLines = 2000
)

// FileChange is review evidence for one completed local write or edit. It is
// saved with activity, outside model history, and never reads live workspace data.
type FileChange struct {
	Path        axlr.RelativePath
	Before      axlr.Text
	After       axlr.Text
	Created     bool
	Unavailable string
}

func (c FileChange) Validate() error {
	if _, err := axlr.NewRelativePath(string(c.Path)); err != nil {
		return err
	}
	if c.Unavailable != "" && c.Unavailable != "too_large" && c.Unavailable != "unavailable" {
		return errors.New("invalid change preview status")
	}
	if c.Unavailable != "" && (c.Before != "" || c.After != "") {
		return errors.New("unavailable change must not retain partial content")
	}
	if c.Created && c.Before != "" {
		return errors.New("created file must not have previous content")
	}
	for _, text := range []axlr.Text{c.Before, c.After} {
		if !ChangePreviewFits(string(text)) {
			return errors.New("change preview exceeds review limits")
		}
		if _, err := axlr.NewText(string(text)); err != nil {
			return err
		}
	}
	return nil
}

func ChangePreviewFits(text string) bool {
	return len(text) <= MaxChangePreviewBytes && strings.Count(text, "\n") < MaxChangePreviewLines
}

func cloneOutcome(outcome ToolOutcome) ToolOutcome {
	if outcome.Change != nil {
		change := *outcome.Change
		outcome.Change = &change
	}
	return outcome
}
