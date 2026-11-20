package application

import (
	"encoding/json"
	"path/filepath"
	"regexp"
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
}

var readOnlyGit = map[string]bool{"status": true, "log": true, "diff": true, "show": true, "rev-parse": true, "ls-files": true, "blame": true, "describe": true, "grep": true, "shortlog": true}

var shellControl = map[string]bool{"for": true, "while": true, "until": true, "if": true, "elif": true, "case": true, "esac": true, "done": true, "fi": true, "in": true, "{": true, "}": true, "(": true, ")": true}

// shellWrite is an output redirection that is not to a discard device or the
// temporary directory: what is left after those are removed.
var (
	shellHarmlessRedirect = regexp.MustCompile(`(?:[0-9]*|&)>>?\s*(?:&[0-9]|/dev/(?:null|stdout|stderr|tty)|/(?:var/)?tmp/[^\s;|&)<>]*)`)
	shellSegment          = regexp.MustCompile(`&&|\|\||[;|&\n]`)
)

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
	if len(args) >= 1 && len(args) <= 2 {
		for _, arg := range args {
			if arg == "--version" || arg == "-V" || arg == "version" || arg == "--help" {
				return true
			}
		}
	}
	switch base {
	case "command":
		return len(args) > 0 && (args[0] == "-v" || args[0] == "-V")
	case "env":
		return len(args) == 0
	case "find":
		for _, arg := range args {
			if arg == "-delete" || strings.HasPrefix(arg, "-exec") || strings.HasPrefix(arg, "-ok") || strings.HasPrefix(arg, "-fprint") {
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
				return readOnlyGit[arg]
			}
		}
		return false
	case "go":
		return len(args) > 0 && (args[0] == "version" || args[0] == "env" || args[0] == "list")
	case "sort":
		for _, arg := range args {
			if arg == "-o" || strings.HasPrefix(arg, "--output") {
				return false
			}
		}
	}
	return readOnlyPrograms[base]
}

// readOnlyScript reports a shell string whose every command only reads or
// prints and that writes no file outside the discard devices and /tmp.
func readOnlyScript(script string) bool {
	rest := shellHarmlessRedirect.ReplaceAllString(script, " ")
	if strings.Contains(rest, ">") || strings.Contains(rest, "<<") {
		return false
	}
	seen := false
	for _, segment := range shellSegment.Split(rest, -1) {
		words := strings.Fields(segment)
		for len(words) > 0 && (shellControl[words[0]] || words[0] == "do" || words[0] == "then" || words[0] == "else" || strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "=")) {
			if words[0] == "for" || words[0] == "case" || words[0] == "in" {
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
