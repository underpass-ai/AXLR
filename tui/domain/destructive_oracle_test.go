package domain

import (
	"strings"
	"testing"
	"time"
)

// oldDestructiveScript is the shell-string check of 0.20.12, kept as an
// oracle: every script it gave a card still gets one, but for the
// relaxations listed in TestScriptCardsAreASupersetOfTheOldOnes.
func oldDestructiveScript(workspace Workspace, script string, rules map[string]bool) string {
	script = discardRedirect.ReplaceAllString(script, " ")
	script = withoutScratchRedirects(script)
	lower := " " + strings.ToLower(strings.Join(strings.Fields(script), " ")) + " "
	for _, rule := range []struct{ name, pattern string }{{RuleGitPush, "git push"}, {RuleGitResetHard, "reset --hard"}, {RuleGitClean, "git clean"}, {RuleGitBranchDelete, "branch -d"}, {RuleGitBranchDelete, "branch --delete"}, {RuleGitRebase, "git rebase"}} {
		if rules[rule.name] && strings.Contains(lower, rule.pattern) {
			return rule.name
		}
	}
	if rules[RuleNetworkWrite] {
		for _, pattern := range []string{"curl -x post", "curl -x put", "curl -x delete", "curl -x patch", "--data", "-d '", "-d \"", "--upload-file", "wget --post", "gh pr create", "gh pr merge", "gh release", "npm publish", "cargo publish", "docker push", "kubectl apply", "kubectl delete", "terraform apply", "terraform destroy", "helm install", "helm upgrade"} {
			if strings.Contains(lower, pattern) {
				return RuleNetworkWrite
			}
		}
	}
	if rules[RuleDeleteOutside] {
		for _, pattern := range []string{"rm -rf /", "rm -rf ~", "rm -r /", "rm -r ~", "rm -rf $home", "> /", ">/", "> ~", ">~", "mv /", "mv ~"} {
			if strings.Contains(lower, pattern) && !strings.Contains(lower, string(workspace)) {
				return RuleDeleteOutside
			}
		}
		for _, field := range strings.Fields(script) {
			if (strings.HasPrefix(field, "/") || strings.HasPrefix(field, "~")) && strings.Contains(lower, " rm ") && outsideWorkspace(workspace, "", field) {
				return RuleDeleteOutside
			}
		}
	}
	return ""
}

// TestScriptCardsAreASupersetOfTheOldOnes: judging a script command by
// command must not lose a card the patterns gave (advisor, 21 November
// 2026), but for a path a command only reads.
func TestScriptCardsAreASupersetOfTheOldOnes(t *testing.T) {
	ws := Workspace("/home/me/repo")
	relaxed := map[string]bool{
		"rm -f out.txt; cat /etc/os-release":      true,
		"rm -rf build && ls /usr/local/bin":       true,
		"rm -rf build; grep -c x /var/log/syslog": true,
	}
	// Cards neither check gave before the review, which the judge now gives.
	for _, script := range []string{
		"env -C /etc rm -rf passwd",
		"sudo -D /etc rm -f passwd",
		"T=/var/log/app; find \"$T\" -delete",
		"CACHE=\"$HOME/.cache/foo\"; rm -rf \"$CACHE\"",
		"$(rm -rf /etc/$((1+1)))",
	} {
		if got := destructiveScript(ws, script, "", map[string]bool{RuleDeleteOutside: true}, false); got == "" {
			t.Errorf("%q has no card", script)
		}
	}
	corpus := []string{
		"(rm -rf /etc)",
		"(cd /tmp && rm -rf x)",
		"timeout 60 rm -rf /etc",
		"exec rm -rf /etc",
		"eval \"rm -rf /etc\"",
		"sudo bash -c \"rm -rf /etc\"",
		"sh -c 'rm -rf /etc'",
		"for d in /etc /usr; do rm -rf $d; done",
		"rm -rf \\\n /etc",
		"echo \"rm -rf /etc\" | sh",
		"rm -rf ~/x",
		"rm -r /var/lib/x",
		"rm -rf $HOME/.config",
		"rm -rf \"$HOME\"/x",
		"rm -rf ${HOME}",
		"echo x > ~/.bashrc",
		"echo x >~/.bashrc",
		"mv /etc/a b",
		"mv ~/a b",
		"cat a > /etc/x",
		"printf x >/etc/x",
		"nohup rm -rf /x &",
		"ls /etc | xargs rm -f",
		"find / -name x -exec rm {} \\;",
		"DIR=/etc; rm -rf $DIR",
		"cd ~ && rm -rf proj",
		"command rm -rf /etc",
		"env rm -rf /x",
		"if true; then rm -rf /etc; fi",
		"while read f; do rm -f /var/log/$f; done < list",
		"time rm -rf /etc",
		"{ rm -rf /etc; }",
		"rm -rf /etc/* 2>/dev/null",
		"ionice -c3 rm -rf /etc",
		"stdbuf -oL rm -rf /etc",
		"rm -rf -- /etc",
		"sudo -u x rm -rf /etc",
		"true && rm -rf /etc || true",
		"rm\t-rf /etc",
		"git push",
		"cd sub && git push origin main",
		"curl -X POST https://api",
		"rm -f out.txt; cat /etc/os-release",
		"rm -rf build && ls /usr/local/bin",
		"rm -rf build; grep -c x /var/log/syslog",
		"go test ./... > /home/me/repo/out.txt",
		"echo x > /tmp/x; rm -rf /tmp/x",
		"cd /home/me/repo && rm -rf /etc",
		// The review of block 8 (21 November 2026): cards 0.20.13 lost.
		"rm -rf $(find /var/log/app -name '*.log' -mtime +7)",
		"rm -f $(ls -d /etc/*.bak)",
		"cd \"$(dirname /etc/passwd)\" && rm -f passwd",
		"sudo bash -c 'cd /var/www && rm -rf *'",
		"bash -c \"cd /etc && rm -f passwd\"",
		"ssh host \"cd /srv && rm -rf old\"",
		"find /etc -type d -name build | xargs -I{} sh -c 'cd {} && rm -rf out'",
		"eval 'cd /etc; rm passwd'",
		"f() { cd /etc; rm passwd; }",
		"builtin cd /etc && rm passwd",
		"env -C /etc rm -rf passwd",
		"sudo -D /etc rm -f passwd",
		"echo $(rm -rf /etc; echo \"(done)\")",
		"x=$(rm -rf /var/lib/app && echo \"(ok)\")",
		"find -L /etc -name x -exec rm {} +",
		"T=/var/log/app; find \"$T\" -delete",
		"CACHE=\"$HOME/.cache/foo\"; rm -rf \"$CACHE\"",
	}
	all := map[string]bool{}
	for _, rule := range DestructiveRules() {
		all[rule] = true
	}
	for _, script := range corpus {
		old := oldDestructiveScript(ws, script, all)
		now := destructiveScript(ws, script, "", all, false)
		if old != "" && now == "" && !relaxed[script] {
			t.Errorf("%q lost its card (%s)", script, old)
		}
		if old == "" && now == "" && strings.Contains(script, "$(dirname /etc") {
			t.Errorf("%q has no card", script)
		}
		if relaxed[script] && now != "" {
			t.Errorf("%q still asks (%s)", script, now)
		}
	}
}

// A long string is judged in time linear in its substitutions' depth, not
// their number: a 300 KB heredoc of Go struct tags took 12 s (review,
// 21 November 2026).
func TestALongScriptIsJudgedQuickly(t *testing.T) {
	script := "cat > tags.go <<'EOF'\n" + strings.Repeat("\tName string `json:\"name\"`\n", 8000) + "EOF\ngo build ./..."
	start := time.Now()
	if got := DestructiveReason("/home/me/repo", "sh", []string{"-c", script}, "", nil); got != "" {
		t.Fatalf("got %q", got)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("judging a %d-byte script took %v", len(script), took)
	}
}
