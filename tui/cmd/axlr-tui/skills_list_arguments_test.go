package main

import (
	"strings"
	"testing"
)

func TestSkillsListRejectsPositionalArguments(t *testing.T) {
	env := skillsEnv(t, nil)
	for _, args := range [][]string{
		{"list", "unexpected"},
		{"list", "--refresh", "unexpected"},
		{"list", "unexpected", "--refresh"},
	} {
		code, stdout, stderr := runSkills(t, env, args...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "usage: axlr-tui skills list [--refresh]") {
			t.Errorf("args %v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}
