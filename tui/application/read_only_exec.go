package application

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
)

// readOnlyPrograms only read or print: running one is no work worth a
// memory. find, git and go are judged by their arguments.
var readOnlyPrograms = map[string]bool{
	"cat": true, "ls": true, "head": true, "tail": true, "wc": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
	"stat": true, "file": true, "du": true, "df": true, "which": true, "type": true, "command": true, "echo": true, "printf": true,
	"pwd": true, "env": true, "printenv": true, "uname": true, "date": true, "id": true, "whoami": true, "hostname": true,
	"tree": true, "diff": true, "cmp": true, "sha256sum": true, "sha1sum": true, "md5sum": true, "uniq": true, "cut": true,
	"tr": true, "nl": true, "realpath": true, "readlink": true, "basename": true, "dirname": true, "test": true, "[": true,
	"true": true, "false": true, "nproc": true, "free": true, "lscpu": true, "ps": true, "sort": true, "jq": true, "read": true,
	"uptime": true, "w": true, "who": true, "groups": true, "tty": true, "arch": true, "locale": true, "getconf": true,
	"lsblk": true, "lspci": true, "lsusb": true, "lsmod": true, "pgrep": true, "pidof": true, "ss": true, "netstat": true,
	"lsof": true, "vmstat": true, "od": true, "hexdump": true, "strings": true, "column": true,
}

// readOnlySystemctl are the systemctl verbs that only show units.
var readOnlySystemctl = map[string]bool{
	"status": true, "show": true, "cat": true, "is-active": true, "is-enabled": true, "is-failed": true, "list-units": true,
	"list-unit-files": true, "list-timers": true, "list-sockets": true, "list-dependencies": true,
}

// writingIP are the ip words that change the network or run a command.
var writingIP = map[string]bool{
	"add": true, "del": true, "delete": true, "set": true, "change": true, "replace": true, "flush": true, "append": true,
	"prepend": true, "exec": true, "attach": true, "detach": true,
}

// versionBump are the options with which `version` writes a new one:
// yarn version --patch bumps, commits and tags.
var versionBump = map[string]bool{"--patch": true, "--minor": true, "--major": true, "--premajor": true, "--preminor": true, "--prepatch": true, "--prerelease": true, "--new-version": true}

var readOnlyGit = map[string]bool{"status": true, "log": true, "diff": true, "show": true, "rev-parse": true, "ls-files": true, "blame": true, "describe": true, "grep": true, "shortlog": true}

var shellControl = map[string]bool{"for": true, "while": true, "until": true, "if": true, "elif": true, "case": true, "esac": true, "done": true, "fi": true, "in": true, "{": true, "}": true, "(": true, ")": true}

// shellHarmlessRedirect is an output redirection to a descriptor, a whole
// discard device or a plain /tmp path (letters, digits and ._-+@%,=:/),
// ended by a blank, a separator or the end; harmlessPath refuses a ".."
// segment. /dev/ttyUSB0 and /tmp/../home/me/notes are writes.
var (
	shellHarmlessRedirect = regexp.MustCompile(`(?:[0-9]*|&)>>?\s*(&[0-9]|/dev/(?:null|stdout|stderr|tty)|/(?:var/)?tmp/[A-Za-z0-9._+@%,=:/-]*)(?:[\s;|&)]|$)`)
	shellSegment          = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	// shellSubstitution is an innermost $(…), <(…), >(…) or `…`: its
	// command runs too.
	shellSubstitution = regexp.MustCompile("[$<>]\\(([^()]*)\\)|`([^`]*)`")
)

// withoutHarmlessRedirects is script with its harmless redirections replaced
// by a blank, keeping the separator that ended each.
func withoutHarmlessRedirects(script string) string {
	return shellHarmlessRedirect.ReplaceAllStringFunc(script, func(match string) string {
		target := shellHarmlessRedirect.FindStringSubmatch(match)[1]
		for _, segment := range strings.Split(target, "/") {
			if segment == ".." {
				return match
			}
		}
		return " " + match[strings.Index(match, target)+len(target):]
	})
}

// readOnlyExec reports a local_exec that only reads or prints: cat, ls, a
// --version probe, git status, or a shell string made of such commands. The
// memory reminder took every command for real work, so a turn that ran only
// cat cost a local model one more request and left a trivial memory in KMP
// (wrx80, 20 November 2026). Unknown programs count as work.
func readOnlyExec(arguments root.JSONValue) bool {
	arguments, _ = normalizeExec(arguments)
	var call struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
		Stdin   string   `json:"stdin"`
	}
	if json.Unmarshal(arguments.Bytes(), &call) != nil || call.Program == "" {
		return false
	}
	base := filepath.Base(call.Program)
	switch base {
	case "sh", "bash", "zsh", "dash":
		for i, arg := range call.Args {
			if strings.HasPrefix(arg, "-") && strings.Contains(arg, "c") && i+1 < len(call.Args) {
				return readOnlyScript(call.Args[i+1])
			}
		}
		return false
	}
	return readOnlyCommand(base, call.Args)
}

func readOnlyCommand(base string, args []string) bool {
	// A probe: --version, -V or --help among one or two arguments, or the
	// word version alone or before an option (npm version, kubectl version
	// --client). npm version patch, rm -rf version and git tag version
	// write (21 November 2026).
	if len(args) >= 1 && len(args) <= 2 {
		for _, arg := range args {
			if arg == "--version" || arg == "-V" || arg == "--help" {
				return true
			}
		}
		if args[0] == "version" && (len(args) == 1 || strings.HasPrefix(args[1], "-") && !versionBump[strings.SplitN(args[1], "=", 2)[0]]) {
			return true
		}
	}
	switch base {
	case "command":
		return len(args) > 0 && (args[0] == "-v" || args[0] == "-V")
	case "env":
		return len(args) == 0
	case "find":
		for _, arg := range args {
			if arg == "-delete" || arg == "-fls" || strings.HasPrefix(arg, "-exec") || strings.HasPrefix(arg, "-ok") || strings.HasPrefix(arg, "-fprint") {
				return false
			}
		}
		return true
	case "git":
		for i := 0; i < len(args); i++ {
			switch arg := args[i]; {
			case arg == "-C" || arg == "-c" || arg == "--git-dir" || arg == "--work-tree":
				i++ // the option's value
			case !strings.HasPrefix(arg, "-"):
				return readOnlyGit[arg] && !slices.ContainsFunc(args[i+1:], func(a string) bool { return strings.HasPrefix(a, "--output") })
			}
		}
		return false
	case "go":
		if len(args) > 0 && args[0] == "env" {
			return !slices.ContainsFunc(args[1:], func(a string) bool { return a == "-w" || a == "-u" })
		}
		return len(args) > 0 && (args[0] == "version" || args[0] == "list")
	case "sort", "tree":
		for _, arg := range args {
			// -o FILE, alone or in a cluster such as sort -uo.
			if strings.HasPrefix(arg, "--output") || len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg, 'o') {
				return false
			}
		}
	case "ss":
		// ss -K kills the sockets it lists.
		return !slices.ContainsFunc(args, func(a string) bool {
			return a == "--kill" || len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a, 'K')
		})
	case "ip":
		return len(args) > 0 && !slices.ContainsFunc(args, func(a string) bool { return writingIP[a] })
	case "systemctl":
		// No verb lists the units; the first word that is no option is the verb.
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				return readOnlySystemctl[arg]
			}
		}
		return true
	case "journalctl":
		return !slices.ContainsFunc(args, func(a string) bool {
			for _, option := range []string{"--vacuum", "--rotate", "--flush", "--sync", "--relinquish-var", "--setup-keys", "--update-catalog"} {
				if strings.HasPrefix(a, option) {
					return true
				}
			}
			return false
		})
	case "uniq":
		// uniq IN OUT writes OUT.
		operands := 0
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				operands++
			}
		}
		if operands > 1 {
			return false
		}
	}
	return readOnlyPrograms[base]
}

// readOnlyScript reports a shell string whose every command only reads or
// prints and that writes no file outside the discard devices and /tmp.
func readOnlyScript(script string) bool {
	// What a command substitution runs counts as a command: echo $(rm -rf
	// build) is a write (21 November 2026). Innermost first; one left
	// unparsed is not judged read-only.
	for depth := 0; depth < 64; depth++ {
		// Every substitution of a depth in one pass, not a pass per match.
		writes := false
		next := shellSubstitution.ReplaceAllStringFunc(script, func(match string) string {
			groups := shellSubstitution.FindStringSubmatch(match)
			if inner := groups[1] + groups[2]; !writes && strings.TrimSpace(inner) != "" && !readOnlyScript(inner) {
				writes = true
			}
			return "x"
		})
		if writes {
			return false
		}
		if next == script {
			break
		}
		script = next
	}
	if strings.Contains(script, "$(") || strings.Contains(script, "<(") || strings.Contains(script, ">(") || strings.Contains(script, "`") {
		return false
	}
	rest := withoutHarmlessRedirects(script)
	if strings.Contains(rest, ">") || strings.Contains(rest, "<<") {
		return false
	}
	seen := false
	for _, segment := range shellSegment.Split(rest, -1) {
		words := strings.Fields(segment)
		for len(words) > 0 && (shellControl[words[0]] || words[0] == "do" || words[0] == "then" || words[0] == "else" || strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "=")) {
			if words[0] == "case" {
				// case WORD in PATTERN) COMMAND: the arm's command runs.
				in := slices.Index(words, "in")
				if in < 0 {
					words = nil
					break
				}
				words = words[in+1:]
				if len(words) > 0 && strings.HasSuffix(words[0], ")") {
					words = words[1:]
				}
				continue
			}
			if words[0] == "for" || words[0] == "in" {
				words = nil // the loop's word list, not a command
				break
			}
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		seen = true
		if !readOnlyCommand(filepath.Base(words[0]), words[1:]) {
			return false
		}
	}
	return seen
}
