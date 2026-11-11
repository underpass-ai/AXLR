package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxMemoryTeamRunes bounds a team's name: it is part of every about the
// team's members write.
const maxMemoryTeamRunes = 64

// MemoryTeam names the team an execution shares its memory with
// (docs/runtime-contract.md, "Policy per execution"): the program that
// runs the execution gives every member of the team one memory store, and
// the name opens their abouts in it, project:<team> for what is the
// team's and project:<team>.<member> for each member's own. So the name
// takes no whitespace or control character, which no about takes, and
// none of ".", ":", "/" and "\", which would read as another team's
// member or as another kind of about.
type MemoryTeam string

func NewMemoryTeam(name string) (MemoryTeam, error) {
	team := MemoryTeam(name)
	return team, team.Validate()
}

func (t MemoryTeam) Validate() error {
	if t == "" {
		return errors.New("the memory team needs a name")
	}
	if !utf8.ValidString(string(t)) || utf8.RuneCountInString(string(t)) > maxMemoryTeamRunes {
		return errors.New("the memory team's name must be valid text of at most 64 characters")
	}
	if strings.IndexFunc(string(t), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 || strings.ContainsAny(string(t), ".:/\\") {
		return errors.New("the memory team's name takes no whitespace, control character, \".\", \":\", \"/\" or \"\\\"")
	}
	return nil
}
