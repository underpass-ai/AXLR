package application

import (
	"errors"
	"strings"
)

// /memory (decisions A4, D7, E15 and I3 of the lifecycle integration
// design): the person's three memory commands. save exports the project
// about as the repository's bundle, compare reads KMP's proposals between
// the session's about and others, forget closes an entry with a
// supersedes relation and never removes it. The person has no write
// command: nothing reaches the project about without evidence.

// ForgetWhy is the reason recorded when the person gives none.
const ForgetWhy = "the person asked the console to forget it"

// RefAbout reads the about a memory ref belongs to: KMP returns refs as
// <about>:entry:<kind>:<slug>. The ref itself is copied, never built.
func RefAbout(ref string) (string, error) {
	about, _, found := strings.Cut(strings.TrimSpace(ref), ":entry:")
	if !found || about == "" {
		return "", errors.New("a memory ref reads <about>:entry:<kind>:<slug>, as the memory engine returns it")
	}
	return about, nil
}
