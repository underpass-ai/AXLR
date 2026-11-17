package main

import (
	"strings"
	"testing"
)

func TestSkillsSearchRequiresTerms(t *testing.T) {
	env := skillsEnv(t, nil)
	for _, args := range [][]string{{"search"}, {"search", "--refresh"}} {
		code, stdout, stderr := runSkills(t, env, args...)
		if code != 2 || !strings.Contains(stderr, "search term is required") || stdout != "" {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	code, stdout, stderr := runSkills(t, env, "search", "missing")
	if code != 0 || !strings.Contains(stdout, "no catalog skill matches") {
		t.Errorf("valid search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
