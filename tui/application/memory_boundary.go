package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session under an execution's policy is a bot's run, not a person's
// (docs/runtime-contract.md, "Policy per execution"): the program that
// runs it decided what it may do. Its memory is bound the same way (8
// November 2026, the QA of the bots' memory): the bot reads and writes
// only its own abouts, the person's about user:<login> neither reaches its
// prompt nor takes what it settles, and a new task wakes what the bot's
// previous task recorded, which a person's session only does when resumed.
//
// A policy that names a memory team (10 November 2026: the store one bot
// had to itself kept qa-jefe from reading what qa-explorador recorded)
// shares the execution's memory with it: the store the program gave is the
// team's, so the bot reads every about in it and writes two, its own,
// project:<team>.<workspace name>, and the team's, project:<team>, beside
// its ws: about. It never writes a teammate's about, so what a memory
// says is always its author's, and each member has its own agent in the
// store.

// memoryBound reports whether the session's memory is bound to its own
// abouts: it runs under an execution's policy.
func memoryBound(s domain.Session) bool {
	_, has := s.Policy()
	return has
}

// memoryBoundState is memoryBound on an exported state.
func memoryBoundState(state domain.SessionState) bool { return state.Policy != nil }

// memoryTeam is the team the session's policy shares its memory with.
func memoryTeam(state domain.SessionState) (domain.MemoryTeam, bool) {
	if state.Policy == nil {
		return "", false
	}
	return state.Policy.MemoryTeam()
}

// teamAbout is what a team's members hold in common, project:<team>.
func teamAbout(team domain.MemoryTeam) string { return "project:" + string(team) }

// teamMember is the session's name in its team: the workspace's, since a
// node gives each bot one workspace named after it; "" when the session
// shares its memory with no team or the workspace has no usable name.
func teamMember(state domain.SessionState) string {
	if _, shared := memoryTeam(state); !shared {
		return ""
	}
	return strings.TrimPrefix(ProjectAbout(state.Workspace), "project:")
}

// MemberAbout is a member's own about in its team's store,
// project:<team>.<member>, the member being the workspace's name as
// ProjectAbout reads it, and bounded as ProjectAbout is; "" for a
// workspace with no usable name. KMP takes no path separator in an about,
// so the member follows a dot.
func MemberAbout(team domain.MemoryTeam, workspace domain.Workspace) string {
	member := strings.TrimPrefix(ProjectAbout(workspace), "project:")
	if member == "" {
		return ""
	}
	about := teamAbout(team) + "." + member
	if utf8.RuneCountInString(about) > 200 {
		about = string([]rune(about)[:200])
	}
	return about
}

// boundAbouts are the abouts a bound session may write, and the ones it
// may read when it shares its memory with no team: the workspace's
// project about and its own ws: about, and its team's between them. The
// selected about (axlr_session) is not consulted: axlr_session is a read
// the policy allows, and a bot that selected another's about would have
// moved its boundary itself.
func boundAbouts(state domain.SessionState) []string {
	about, own := defaultAbout(state), sessionAbout(state.ID)
	abouts := []string{about}
	if team, shared := memoryTeam(state); shared {
		abouts = append(abouts, teamAbout(team))
	}
	if about != own {
		abouts = append(abouts, own)
	}
	return abouts
}

// sessionMemoryAbout is where a session's memory goes when no about is
// named: the workspace's project about for a bound session, else the
// selected about or the project's (memoryAbout).
func sessionMemoryAbout(ctx context.Context, labels SessionLabelsPort, state domain.SessionState) (string, error) {
	if memoryBoundState(state) {
		return defaultAbout(state), nil
	}
	about, _, err := memoryAbout(ctx, labels, state)
	return about, err
}

// checkBoundAbout refuses a write to an about outside a bound session's
// own; its refusal starts with the abouts it may name.
func checkBoundAbout(session domain.Session, about string) error {
	return boundAboutRefusal(session, about, func(allowed []string) string {
		return "name about as one of " + strings.Join(allowed, ", ")
	})
}

// checkRememberAbout is checkBoundAbout for axlr_remember, whose refusal
// starts with leaving about out: on 12 November 2026 a Codex model that
// had invented an about read a refusal that began with the policy and
// sent another invented one, ten times.
func checkRememberAbout(session domain.Session, about string) error {
	return boundAboutRefusal(session, about, func(allowed []string) string {
		return fmt.Sprintf("leave about out to write under %s, or name one of %s", allowed[0], strings.Join(allowed, ", "))
	})
}

// boundAboutRefusal refuses an about outside a bound session's own: what
// to use first, from use, then why this one is refused.
func boundAboutRefusal(session domain.Session, about string, use func(allowed []string) string) error {
	if about == "" || !memoryBound(session) {
		return nil
	}
	state := session.Export()
	allowed := boundAbouts(state)
	for _, own := range allowed {
		if about == own {
			return nil
		}
	}
	if _, shared := memoryTeam(state); shared {
		return fmt.Errorf("%s: %s is a teammate's or another's, which the execution's policy lets this session read and not write", use(allowed), about)
	}
	return fmt.Errorf("%s: the execution's policy keeps memory to its own abouts and %s is another's", use(allowed), about)
}

// checkBoundRead refuses a read of an about outside a bound session's
// own. A session that shares its memory with a team reads every about:
// the store it opened is the team's, and nobody else's memory is in it.
func checkBoundRead(session domain.Session, about string) error {
	if _, shared := memoryTeam(session.Export()); shared {
		return nil
	}
	return checkBoundAbout(session, about)
}

// checkMemoryWrite refuses the engine's memory write, called by the model
// itself, when it names an about a team's member may not write. Without a
// team the store is the bot's alone and the call goes as it came, as
// before; a continuation names no about and commits a write this check
// already saw.
func checkMemoryWrite(session domain.Session, tool domain.AvailableTool, arguments root.JSONValue) error {
	if tool.Identity.Kind != domain.ToolKindPlugin || !tool.Surface.MemoryWrite {
		return nil
	}
	if _, shared := memoryTeam(session.Export()); !shared {
		return nil
	}
	var call struct {
		About        string `json:"about"`
		Continuation string `json:"continuation"`
	}
	if err := json.Unmarshal(arguments.Bytes(), &call); err != nil {
		return errors.New("the memory write's arguments are not an object with a text about")
	}
	if call.About == "" {
		if call.Continuation != "" {
			return nil
		}
		return fmt.Errorf("name about as one of %s: the execution's policy keeps what it writes to its own abouts and its team's", strings.Join(boundAbouts(session.Export()), ", "))
	}
	return checkBoundAbout(session, call.About)
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
