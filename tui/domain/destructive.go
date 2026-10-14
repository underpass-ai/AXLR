package domain

import (
	"path/filepath"
	"strings"
)

// The destructive list (decision K5a of the lifecycle integration design):
// what keeps an approval card under full autonomy. AXLR maintains the list
// in settings (approvals.always_ask names the rules in force) and applies
// it to local_exec by program and arguments.

// Destructive rule names, as settings list them.
const (
	RuleDeleteOutside   = "delete-outside-workspace"
	RuleGitPush         = "git-push"
	RuleGitResetHard    = "git-reset-hard"
	RuleGitClean        = "git-clean"
	RuleGitBranchDelete = "git-branch-delete"
	RuleGitRebase       = "git-rebase"
	RuleNetworkWrite    = "network-write"
)

// DestructiveRules lists every rule, in force by default.
func DestructiveRules() []string {
	return []string{RuleDeleteOutside, RuleGitPush, RuleGitResetHard, RuleGitClean, RuleGitBranchDelete, RuleGitRebase, RuleNetworkWrite}
}

// deleters are programs whose path arguments remove or overwrite.
var deleters = map[string]bool{"rm": true, "rmdir": true, "unlink": true, "shred": true, "mv": true, "cp": true, "dd": true, "truncate": true, "chmod": true, "chown": true}

// fileRemovers delete, move or cut files whatever path they name.
var fileRemovers = map[string]bool{"rm": true, "rmdir": true, "unlink": true, "shred": true, "mv": true, "truncate": true}

// RemovesFiles reports a command that deletes, moves or truncates files,
// behind sudo, env or nice too, with at least one argument that is not a
// flag. It is no rule: the console warns on the command's card, since
// undo restores only what the session's writes and edits changed. On
// 14 October 2026 glm-5.3-flash deleted calc/calc.go "to test path
// resolution" and its card read like any command.
func RemovesFiles(program string, args []string) bool {
	program, args = unwrapPrivilege(program, args)
	if !fileRemovers[filepath.Base(program)] {
		return false
	}
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// shells run a command string; their string is matched literally.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true, "ksh": true}

// networkWriters are programs whose named subcommands or flags have an
// effect beyond this machine.
var networkWriters = map[string][]string{
	"gh":        {"pr create", "pr merge", "pr close", "release", "repo create", "repo delete", "api -X POST", "api -X PUT", "api -X DELETE", "api -X PATCH", "api --method"},
	"npm":       {"publish", "unpublish", "deprecate"},
	"cargo":     {"publish", "yank"},
	"twine":     {"upload"},
	"docker":    {"push"},
	"kubectl":   {"apply", "delete", "rollout", "scale"},
	"helm":      {"install", "upgrade", "uninstall", "rollback"},
	"terraform": {"apply", "destroy"},
	"flyctl":    {"deploy"},
	"fly":       {"deploy"},
	"vercel":    {"deploy", "--prod"},
	"aws":       {"deploy", "s3 rm", "s3 sync", "s3 cp"},
	"gcloud":    {"deploy", "delete"},
}

// DestructiveReason names the rule a command falls under, or "" when none
// of the enabled rules does. program and args are the call's; cwd is the
// workspace-relative directory it runs in; enabled names the rules in
// force (nil: every rule).
func DestructiveReason(workspace Workspace, program string, args []string, cwd string, enabled []string) string {
	rules := map[string]bool{}
	if enabled == nil {
		enabled = DestructiveRules()
	}
	for _, rule := range enabled {
		rules[rule] = true
	}
	program, args = unwrapPrivilege(program, args)
	base := filepath.Base(program)
	if shells[base] {
		if script := shellScript(args); script != "" {
			return destructiveScript(workspace, script, rules)
		}
	}
	words := strings.Join(append([]string{base}, args...), " ")
	if rule := gitRule(base, args); rule != "" && rules[rule] {
		return rule
	}
	if rules[RuleNetworkWrite] && networkWrite(base, args, words) {
		return RuleNetworkWrite
	}
	if rules[RuleDeleteOutside] && deleters[base] {
		for _, arg := range args {
			if strings.HasPrefix(arg, "-") || arg == "" {
				continue
			}
			if outsideWorkspace(workspace, cwd, arg) {
				return RuleDeleteOutside
			}
		}
	}
	return ""
}

// wrapperValues are the options of each wrapper that take the next word
// as their value: "nice -n 10 rm" runs rm, not 10 (read as the program
// until 14 October 2026).
var wrapperValues = map[string]map[string]bool{
	"sudo":  {"-u": true, "-g": true, "-C": true, "-D": true, "-h": true, "-p": true, "-r": true, "-t": true, "-T": true, "-U": true, "--user": true, "--group": true, "--chdir": true, "--host": true, "--prompt": true, "--role": true, "--type": true, "--other-user": true, "--close-from": true, "--command-timeout": true},
	"doas":  {"-u": true, "-C": true},
	"env":   {"-u": true, "-C": true, "-S": true, "--unset": true, "--chdir": true, "--split-string": true},
	"nice":  {"-n": true, "--adjustment": true},
	"time":  {"-f": true, "-o": true, "--format": true, "--output": true},
	"nohup": {},
}

// unwrapPrivilege strips sudo, doas and env prefixes.
func unwrapPrivilege(program string, args []string) (string, []string) {
	for {
		base := filepath.Base(program)
		values, wrapper := wrapperValues[base]
		if !wrapper || len(args) == 0 {
			return program, args
		}
		i := 0
		for i < len(args) && (strings.HasPrefix(args[i], "-") || (base == "env" && strings.Contains(args[i], "="))) {
			if values[args[i]] {
				i++ // the option's value
			}
			i++
		}
		if i >= len(args) {
			return program, args
		}
		program, args = args[i], args[i+1:]
	}
}

// shellScript is the -c string of a shell call, "" when it runs a file.
// Any cluster of short flags with c in it counts: sh -xc and bash -euc run
// their string as -c does.
func shellScript(args []string) string {
	for i, arg := range args {
		if commandFlag(arg) && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// commandFlag reports a cluster of short shell flags that holds -c.
func commandFlag(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	for _, r := range arg[1:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return strings.ContainsRune(arg[1:], 'c')
}

// destructiveScript matches a shell string literally: a card for a false
// positive costs a keypress, a missed deletion costs more.
func destructiveScript(workspace Workspace, script string, rules map[string]bool) string {
	lower := " " + strings.ToLower(script) + " "
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

// gitRule names the git rule a call falls under.
func gitRule(base string, args []string) string {
	if base != "git" {
		return ""
	}
	sub := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-C" || arg == "-c" || arg == "--git-dir" || arg == "--work-tree" {
			i++ // the option's value
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			sub = arg
			break
		}
	}
	switch sub {
	case "push":
		return RuleGitPush
	case "reset":
		for _, arg := range args {
			if arg == "--hard" {
				return RuleGitResetHard
			}
		}
	case "clean":
		return RuleGitClean
	case "branch":
		for _, arg := range args {
			if arg == "-D" || arg == "-d" || arg == "--delete" {
				return RuleGitBranchDelete
			}
		}
	case "rebase":
		return RuleGitRebase
	}
	return ""
}

// networkWrite reports a network call with effect.
func networkWrite(base string, args []string, words string) bool {
	switch base {
	case "curl":
		for i, arg := range args {
			switch {
			case arg == "-X" || arg == "--request":
				if i+1 < len(args) && strings.ToUpper(args[i+1]) != "GET" && strings.ToUpper(args[i+1]) != "HEAD" {
					return true
				}
			case strings.HasPrefix(arg, "-X") && len(arg) > 2 && strings.ToUpper(arg[2:]) != "GET":
				return true
			case arg == "-d" || arg == "-F" || arg == "-T" || strings.HasPrefix(arg, "--data") || arg == "--form" || arg == "--upload-file" || strings.HasPrefix(arg, "--json"):
				return true
			}
		}
	case "wget":
		for _, arg := range args {
			if strings.HasPrefix(arg, "--post-") || strings.HasPrefix(arg, "--method=") && !strings.HasSuffix(strings.ToUpper(arg), "=GET") || arg == "--method" {
				return true
			}
		}
	}
	for _, pattern := range networkWriters[base] {
		if strings.Contains(" "+words+" ", " "+pattern+" ") || strings.HasPrefix(words, base+" "+pattern) {
			return true
		}
	}
	return false
}

// outsideWorkspace reports a path that resolves outside the workspace
// root: absolute elsewhere, under the home directory, or climbing out.
func outsideWorkspace(workspace Workspace, cwd, path string) bool {
	root := filepath.Clean(string(workspace))
	if root == "." || root == "" {
		return false
	}
	if strings.HasPrefix(path, "~") {
		return true
	}
	// Commands are POSIX shell words: a leading slash is absolute on every
	// platform (on Windows filepath.IsAbs says otherwise, seen in CI on
	// 13 October 2026).
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		path = filepath.Join(root, cwd, path)
	}
	path = filepath.Clean(path)
	return path != root && !strings.HasPrefix(path, root+string(filepath.Separator))
}
