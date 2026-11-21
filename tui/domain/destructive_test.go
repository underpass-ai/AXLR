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
		{"kubectl", []string{"--context", "lab", "rollout", "status", "-n", "shop", "deploy/web"}, "", ""},
		{"kubectl", []string{"rollout", "history", "deploy/web"}, "", ""},
		{"kubectl", []string{"rollout", "undo", "deploy/web"}, "", RuleNetworkWrite},
		{"kubectl", []string{"rollout", "restart", "deploy/web"}, "", RuleNetworkWrite},
		{"kubectl", []string{"rollout", "-n", "shop", "status", "deploy/web"}, "", RuleNetworkWrite},
		{"kubectl", []string{"rollout"}, "", RuleNetworkWrite},
		{"kubectl", []string{"get", "pods"}, "", ""},
		{"kubectl", []string{"-n", "shop", "scale", "deploy/web", "--replicas=0"}, "", RuleNetworkWrite},
		{"bash", []string{"-c", "git push origin main"}, "", RuleGitPush},
		{"sh", []string{"-c", "rm -rf /tmp/x"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "rm -rf build && go test ./..."}, "", ""},
		{"sh", []string{"-c", "echo hi > /etc/motd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "curl -X DELETE https://api"}, "", RuleNetworkWrite},
		// A redirection to a discard device writes nothing (20 November 2026).
		{"sh", []string{"-c", "for p in apt pacman; do command -v $p >/dev/null 2>&1 && echo $p; done"}, "", ""},
		{"sh", []string{"-c", "ls ~/go/bin 2>/dev/null; nmap -V 2> /dev/null | head -1"}, "", ""},
		{"bash", []string{"-c", "make test &>/dev/null || echo failed >/dev/stderr"}, "", ""},
		{"sh", []string{"-c", "cat x >>/dev/null"}, "", ""},
		{"sh", []string{"-c", "ls 2>/dev/null; echo hi > /etc/motd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo hi >/dev/nullfile"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "ls 2>/dev/null && rm -rf /tmp/x"}, "", RuleDeleteOutside},
		// A redirection into the temporary directory writes a scratch file
		// (20 November 2026); deleting there or writing elsewhere still asks.
		{"sh", []string{"-c", "printf 'a\\nb\\n' > /tmp/counts.txt; grep -c a /tmp/counts.txt"}, "", ""},
		{"bash", []string{"-c", "go test ./... >/var/tmp/out.log 2>&1; tail /var/tmp/out.log"}, "", ""},
		{"sh", []string{"-c", "echo x > /tmp/x; rm -rf /tmp/x"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x > /tmp/x; echo y > /etc/hosts"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x > /tmpfile"}, "", RuleDeleteOutside},
		// Only a plain path stays scratch: one that climbs out of /tmp or
		// hides a command substitution keeps its card (21 November 2026).
		{"sh", []string{"-c", "echo x > /tmp/../etc/passwd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x >/tmp/../home/me/.bashrc"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x > /tmp/$(rm -rf ~)"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x > /tmp/`rm -rf ~`"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x > /tmp/a.log;rm -rf /home/me"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "go vet ./... 2>/tmp/vet-1.txt; wc -l /tmp/vet-1.txt"}, "", ""},
		// A stripped redirection does not split a rule's words (21 November
		// 2026: "git 2>/dev/null push" left "git   push").
		{"sh", []string{"-c", "git 2>/dev/null push origin main"}, "", RuleGitPush},
		{"sh", []string{"-c", "git 2>/tmp/e push --force"}, "", RuleGitPush},
		{"sh", []string{"-c", "git  reset  --hard HEAD~1"}, "", RuleGitResetHard},
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
		// Each command of a script is judged on its own: a script that
		// named the workspace skipped every pattern, so cd into it and
		// echo x > /etc/hosts ran without a card (review, 21 November 2026).
		{"sh", []string{"-c", "cd /home/me/repo && echo x > /etc/hosts"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd /home/me/repo && mv /etc/hosts x"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd /home/me/repo&&rm -rf /etc"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "ls;rm -rf /etc"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd /home/me/repo && echo $(rm -rf ~)"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "diff <(rm -rf ~/notes) /dev/null"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x >| /etc/passwd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x >&/etc/passwd"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "echo x | sudo tee -a /etc/hosts"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "dd if=/dev/zero of=/dev/sda bs=1M"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd .. && rm -rf other"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd .. && rm -rf repo"}, "", RuleDeleteOutside},
		{"rm", []string{"-rf", "/home/me/repo"}, "", RuleDeleteOutside},
		{"rm", []string{"-rf", "/home/me/repo/build"}, "", ""},
		{"find", []string{"/var/log", "-delete"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd / && rm -rf usr"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "cd $HOME; rm -rf notes"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "ls /etc | xargs rm -f"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "find /var/log -name '*.gz' -delete"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "find /etc -name x -exec rm {} \\;"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "rm -rf \"/home/me/other dir\""}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "git -C sub push origin main"}, "", RuleGitPush},
		{"sh", []string{"-c", "git --no-pager push"}, "", RuleGitPush},
		{"sh", []string{"-c", "cd /home/me/repo && git push"}, "", RuleGitPush},
		{"sh", []string{"-c", "cd /home/me/repo && curl -sS -X POST https://api.example"}, "", RuleNetworkWrite},
		// …and only what a command deletes or writes counts: a path it
		// reads elsewhere in the script no longer raises the card.
		{"sh", []string{"-c", "cd /home/me/repo && go test ./... > /home/me/repo/out.txt 2>&1"}, "", ""},
		{"sh", []string{"-c", "rm -f out.txt; cat /etc/os-release"}, "", ""},
		{"sh", []string{"-c", "cd /home/me/repo/sub && rm -rf build"}, "", ""},
		{"sh", []string{"-c", "cd sub && rm -rf ../build"}, "", ""},
		{"sh", []string{"-c", "find . -name '*.o' -delete; ls -la /usr/local/bin"}, "", ""},
		{"sh", []string{"-c", "go build -o bin/x ./cmd/x 2>&1 | tee build.log"}, "", ""},
		{"sh", []string{"-c", "if [ -d build ]; then rm -rf build; fi"}, "", ""},
		{"sh", []string{"-c", "FOO=1 rm -rf build"}, "", ""},
		// tee into scratch or a device, mv into the workspace's root, cp's
		// sources, a search for "rm -rf" and a loop over workspace files
		// write nothing outside.
		{"sh", []string{"-c", "go test ./... 2>&1 | tee /tmp/test.log"}, "", ""},
		{"sh", []string{"-c", "go test ./... | tee /dev/stderr"}, "", ""},
		{"mv", []string{"build/out.txt", "."}, "", ""},
		{"sh", []string{"-c", "mv build/out.txt ."}, "", ""},
		{"sh", []string{"-c", "cp /etc/os-release x"}, "", ""},
		{"cp", []string{"/etc/os-release", "x"}, "", ""},
		{"cp", []string{"-t", "/etc", "x"}, "", RuleDeleteOutside},
		{"sh", []string{"-c", "grep -rn \"rm -rf\" ."}, "", ""},
		{"sh", []string{"-c", "for f in *.o; do rm -f $f; done"}, "", ""},
		{"sh", []string{"-c", "tmp=$(mktemp); echo x > $tmp; rm -f $tmp"}, "", ""},
		{"sh", []string{"-c", "echo x | tee -a /etc/hosts"}, "", RuleDeleteOutside},
	}
	for _, tc := range cases {
		if got := DestructiveReason(ws, tc.program, tc.args, tc.cwd, nil); got != tc.want {
			t.Fatalf("%s %v (cwd %q): got %q want %q", tc.program, tc.args, tc.cwd, got, tc.want)
		}
	}
	// A workspace in mixed case is compared as written.
	if got := DestructiveReason("/home/Me/Repo", "sh", []string{"-c", "cd /home/Me/Repo && rm -rf build"}, "", nil); got != "" {
		t.Fatalf("a mixed-case workspace raised %q", got)
	}
	if got := DestructiveReason("/home/Me/Repo", "sh", []string{"-c", "cd /home/Me/Repo && rm -rf /etc"}, "", nil); got != RuleDeleteOutside {
		t.Fatalf("a delete outside a mixed-case workspace raised %q", got)
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
