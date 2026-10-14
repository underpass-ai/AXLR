package domain

import "testing"

func TestDestructiveReasonNamesTheRule(t *testing.T) {
	ws := Workspace("/home/me/repo")
	cases := []struct {
		program string
		args    []string
		cwd     string
		want    string
	}{
		{"git", []string{"push", "origin", "main"}, "", RuleGitPush},
		{"git", []string{"-C", "x", "reset", "--hard", "HEAD~1"}, "", RuleGitResetHard},
		{"git", []string{"clean", "-fd"}, "", RuleGitClean},
		{"git", []string{"branch", "-D", "old"}, "", RuleGitBranchDelete},
		{"git", []string{"rebase", "main"}, "", RuleGitRebase},
		{"git", []string{"status"}, "", ""},
		{"git", []string{"reset", "HEAD", "file"}, "", ""},
		{"rm", []string{"-rf", "/tmp/other"}, "", RuleDeleteOutside},
		{"rm", []string{"-rf", "build"}, "", ""},
		{"rm", []string{"-rf", "../sibling"}, "", RuleDeleteOutside},
		{"rm", []string{"-rf", "../deep"}, "sub", ""},
		{"mv", []string{"a.txt", "~/a.txt"}, "", RuleDeleteOutside},
		{"cp", []string{"a.txt", "/etc/passwd"}, "", RuleDeleteOutside},
		{"sudo", []string{"rm", "-rf", "/var/lib/x"}, "", RuleDeleteOutside},
		{"curl", []string{"-X", "POST", "https://api.example"}, "", RuleNetworkWrite},
		{"curl", []string{"-sS", "https://example.com"}, "", ""},
		{"curl", []string{"--data", "x=1", "https://example.com"}, "", RuleNetworkWrite},
		{"wget", []string{"--post-data=x", "https://example.com"}, "", RuleNetworkWrite},
		{"gh", []string{"pr", "create", "--fill"}, "", RuleNetworkWrite},
		{"gh", []string{"pr", "view", "1"}, "", ""},
		{"npm", []string{"publish"}, "", RuleNetworkWrite},
		{"npm", []string{"test"}, "", ""},
		{"docker", []string{"push", "img"}, "", RuleNetworkWrite},
		{"bash", []string{"-c", "git push origin main"}, "", RuleGitPush},
		{"sh", []string{"-c", "rm -rf /tmp/x"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "rm -rf build && go test ./..."}, "", ""},
		{"sh", []string{"-c", "echo hi > /etc/motd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "curl -X DELETE https://api"}, "", RuleNetworkWrite},
		{"go", []string{"test", "./..."}, "", ""},
		{"ls", []string{"/"}, "", ""},
		// A wrapper option's value is not the program (14 October 2026).
		{"nice", []string{"-n", "10", "rm", "-rf", "/x"}, "", RuleDeleteOutside},
		{"nice", []string{"-n10", "rm", "-rf", "/x"}, "", RuleDeleteOutside},
		{"sudo", []string{"-u", "deploy", "git", "push"}, "", RuleGitPush},
		{"env", []string{"-u", "VAR", "rm", "-rf", "/x"}, "", RuleDeleteOutside},
		{"env", []string{"-u", "VAR", "A=1", "git", "push"}, "", RuleGitPush},
		// A cluster of short flags with c runs its string.
		{"sh", []string{"-xc", "git push origin main"}, "", RuleGitPush},
		{"bash", []string{"-euc", "rm -rf /tmp/x"}, "", RuleDeleteOutside},
		{"bash", []string{"-eu", "script.sh"}, "", ""},
		{"bash", []string{"--rcfile", "x", "script.sh"}, "", ""},
	}
	for _, tc := range cases {
		if got := DestructiveReason(ws, tc.program, tc.args, tc.cwd, nil); got != tc.want {
			t.Fatalf("%s %v (cwd %q): got %q want %q", tc.program, tc.args, tc.cwd, got, tc.want)
		}
	}
	// Only the rules in force apply.
	if got := DestructiveReason(ws, "git", []string{"push"}, "", []string{RuleNetworkWrite}); got != "" {
		t.Fatalf("a rule not in force applied: %q", got)
	}
	if got := DestructiveReason(ws, "git", []string{"push"}, "", []string{}); got != "" {
		t.Fatalf("an empty list applied a rule: %q", got)
	}
	if len(DestructiveRules()) != 7 {
		t.Fatal("rules")
	}
}
