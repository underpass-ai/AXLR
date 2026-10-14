package application

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ProjectAbout is the KMP about the console gives the model's memory by
// default: "project:" and the workspace root's base name with its case,
// with each run of spaces or control characters as one "-"
// ("/home/t/ai/AXLR" gives "project:AXLR", what KMP's own hosts write for
// that checkout). It is derived from the path alone, so every session in
// the same checkout gets the same about without asking KMP or git; a clone
// under another directory name gets its own. It is empty when the root has
// no usable name ("/"). A checkout whose committed bundle names a project
// about gets that one instead, selected for the session at its start.
//
// On 10 October 2026 claude-haiku-5.5 never selected an about, so session A
// recorded under ws:<session A>, and session B in the same workspace looked
// for project:AXLR and project:axlr and answered "nothing recorded". Until
// 13 October 2026 the name was lower-cased, which split this repository's
// memory between project:AXLR (the person's hosts) and project:axlr (the
// console); abouts are case-sensitive, so the case is kept (decision D2 of
// the lifecycle integration design).
func ProjectAbout(workspace domain.Workspace) string {
	name := filepath.Base(filepath.Clean(string(workspace)))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return ""
	}
	var b strings.Builder
	dash := false
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == utf8.RuneError {
			dash = b.Len() > 0
			continue
		}
		if dash {
			b.WriteByte('-')
			dash = false
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return ""
	}
	about := "project:" + b.String()
	if utf8.RuneCountInString(about) > 200 {
		about = string([]rune(about)[:200])
	}
	return about
}

// sessionAbout is a session's own about, ws:<session id>. Console-driven
// ceremonies keep it for their recall and outcomes (decision 2 of the
// ceremony driver design), and it is the default when the workspace has no
// usable name.
func sessionAbout(id domain.SessionID) string { return "ws:" + string(id) }

// memoryAbout is where the model's memory of a session goes when it names no
// about: the about selected for the session (axlr_session about, or the
// person), else the workspace's ProjectAbout. selected reports the first.
func memoryAbout(ctx context.Context, labels SessionLabelsPort, state domain.SessionState) (about string, selected bool, err error) {
	if labels != nil {
		all, err := labels.Load(ctx)
		if err != nil {
			return "", false, err
		}
		if chosen := all[state.ID].About; chosen != "" {
			return chosen, true, nil
		}
	}
	return defaultAbout(state), false, nil
}

// defaultAbout is ProjectAbout, or the session's own about where the
// workspace gives none.
func defaultAbout(state domain.SessionState) string {
	if about := ProjectAbout(state.Workspace); about != "" {
		return about
	}
	return sessionAbout(state.ID)
}
