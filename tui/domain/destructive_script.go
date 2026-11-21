package domain

import (
	"path/filepath"
	"regexp"
	"strings"
)

// A shell string is judged command by command, as a call is: every command
// of every segment, and of every substitution, with the directory a cd
// left it in. The patterns it was matched with before were all skipped
// when the script named the workspace, so `cd <workspace> && echo x >
// /etc/hosts` ran without a card, and an rm anywhere put a card on any
// absolute path a command only read (review of 21 November 2026).

var (
	// scriptSubstitution is an innermost $(…), <(…), >(…) or `…`: its
	// commands run too, in a subshell.
	scriptSubstitution = regexp.MustCompile("[$<>]\\(([^()]*)\\)|`([^`]*)`")
	// scriptRedirect is an output redirection and its target: >, >>, >|,
	// &>, >&FILE. A >& whose target is a descriptor duplicates it.
	scriptRedirect = regexp.MustCompile(`(?:[0-9]*|&)>(?:>|\|)?(&)?\s*([^\s;|&<>()]+)`)
	// scriptSeparator ends a command.
	scriptSeparator = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	// scriptProtected are the operators holding a separator's character.
	scriptProtected = strings.NewReplacer(">&", ">\x00", "&>", "\x00>", ">|", ">\x01")
	scriptRestored  = strings.NewReplacer("\x00", "&", "\x01", "|")
)

// shellKeywords open, close or join commands; they are no program.
var shellKeywords = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true, "do": true, "done": true, "while": true, "until": true, "esac": true, "{": true, "}": true, "(": true, ")": true, "!": true, "time": true}

// writers name the files they write, and so are judged like deleters.
var writers = map[string]bool{"tee": true}

// scriptJudge walks a shell string's commands in order.
type scriptJudge struct {
	workspace Workspace
	rules     map[string]bool
	cwd       string // the workspace-relative directory; may climb out with ..
	lost      bool   // a cd went where the string does not say
	paths     []string
}

func destructiveCommands(workspace Workspace, script, cwd string, rules map[string]bool) string {
	script = strings.ReplaceAll(script, "\\\n", " ") // a continued line is one command
	j := scriptJudge{workspace: workspace, rules: rules, cwd: cwd}
	for _, word := range strings.Fields(script) {
		word = unquote(word)
		if assignment(word) {
			word = unquote(word[strings.IndexByte(word, '=')+1:])
		}
		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") {
			j.paths = append(j.paths, word)
		}
	}
	return j.script(script)
}

func (j *scriptJudge) script(script string) string {
	for {
		m := scriptSubstitution.FindStringSubmatchIndex(script)
		if m == nil {
			break
		}
		inner := *j // a subshell: its cd stays inside
		if rule := inner.script(script[max(m[2], m[4]):max(m[3], m[5])]); rule != "" {
			return rule
		}
		script = script[:m[0]] + "x" + script[m[1]:]
	}
	for _, segment := range scriptSeparator.Split(scriptProtected.Replace(script), -1) {
		if rule := j.segment(scriptRestored.Replace(segment)); rule != "" {
			return rule
		}
	}
	return ""
}

func (j *scriptJudge) segment(segment string) string {
	if j.rules[RuleDeleteOutside] {
		for _, m := range scriptRedirect.FindAllStringSubmatch(segment, -1) {
			target := unquote(m[2])
			if m[1] == "&" && (target == "-" || strings.Trim(target, "0123456789") == "") {
				continue
			}
			if j.outside(target) {
				return RuleDeleteOutside
			}
		}
	}
	raw := strings.Fields(scriptRedirect.ReplaceAllString(segment, " "))
	if len(raw) > 0 && raw[0] == "case" {
		// case WORD in PATTERN) COMMAND: the arm's command runs.
		in := -1
		for i, word := range raw {
			if word == "in" {
				in = i
				break
			}
		}
		if in < 0 {
			return ""
		}
		raw = raw[in+1:]
		if len(raw) > 0 && strings.HasSuffix(raw[0], ")") {
			raw = raw[1:]
		}
	}
	// Quotes and a subshell's parentheses are no part of a word: (rm -rf
	// /etc) runs rm.
	var words []string
	for _, word := range raw {
		if word = strings.Trim(word, "\"'()"); word != "" {
			words = append(words, word)
		}
	}
	for len(words) > 0 && (shellKeywords[words[0]] || words[0] == "for" || assignment(words[0])) {
		if words[0] == "for" {
			return "" // the loop's word list, not a command
		}
		words = words[1:]
	}
	if len(words) == 0 {
		return ""
	}
	if words[0] == "cd" || words[0] == "pushd" {
		j.chdir(words[1:])
		return ""
	}
	if rule := j.command(words[0], words[1:]); rule != "" {
		return rule
	}
	// A remover after a word the judge does not know runs it, or hands it
	// to a shell: timeout 60 rm, exec rm, eval "rm …", sudo bash -c "rm …",
	// echo "rm …" | sh. Its operands are judged as a call's.
	if j.rules[RuleDeleteOutside] {
		for i := 1; i < len(words); i++ {
			if base := filepath.Base(words[i]); fileRemovers[base] || writers[base] || base == "dd" {
				if rule := j.command(words[i], words[i+1:]); rule != "" {
					return rule
				}
			}
		}
	}
	return ""
}

func (j *scriptJudge) command(program string, args []string) string {
	program, args = unwrapPrivilege(program, args)
	base := filepath.Base(program)
	if rule := gitRule(base, args); rule != "" && j.rules[rule] {
		return rule
	}
	if j.rules[RuleNetworkWrite] && networkWrite(base, args, strings.Join(append([]string{base}, args...), " ")) {
		return RuleNetworkWrite
	}
	if !j.rules[RuleDeleteOutside] {
		return ""
	}
	switch base {
	case "xargs":
		// Its paths come from the pipe: any absolute path the string
		// names stands for them.
		if j.removesFromInput(args) {
			for _, path := range j.paths {
				if j.outside(path) {
					return RuleDeleteOutside
				}
			}
		}
		return ""
	case "find":
		if findDeletes(args) {
			for _, arg := range args {
				if strings.HasPrefix(arg, "-") || arg == "(" || arg == "!" {
					break
				}
				if j.outside(arg) {
					return RuleDeleteOutside
				}
			}
		}
		return ""
	case "dd":
		for _, arg := range args {
			if strings.HasPrefix(arg, "of=") && !scratchOrDiscard(arg[3:]) && j.outside(arg[3:]) {
				return RuleDeleteOutside
			}
		}
		return ""
	case "cp":
		// cp writes only its destination; its sources are read.
		if target := copyTarget(args); target != "" && j.outside(target) {
			return RuleDeleteOutside
		}
		return ""
	}
	if !deleters[base] && !writers[base] {
		return ""
	}
	operands := nonFlags(args)
	for i, arg := range operands {
		switch {
		case writers[base] && scratchOrDiscard(arg):
			// tee /tmp/test.log or tee /dev/stderr writes scratch, as a
			// redirection there does.
		case j.outside(arg):
			return RuleDeleteOutside
		case j.unresolved(arg):
			return RuleDeleteOutside
		case removesWhole[base] && (base != "mv" || i < len(operands)-1) && j.isRoot(arg):
			return RuleDeleteOutside
		}
	}
	return ""
}

// removesWhole are the programs that take away what they name: rm .. on
// the workspace removes it. mv moves only its sources away.
var removesWhole = map[string]bool{"rm": true, "rmdir": true, "unlink": true, "shred": true, "mv": true}

// nonFlags are a command's operands: its words but options, and all of
// them after --.
func nonFlags(args []string) []string {
	var operands []string
	for i, arg := range args {
		if arg == "--" {
			return append(operands, args[i+1:]...)
		}
		if arg != "" && !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
		}
	}
	return operands
}

// copyTarget is the file or directory cp writes: -t's value, or its last
// operand.
func copyTarget(args []string) string {
	for i, arg := range args {
		if (arg == "-t" || arg == "--target-directory") && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--target-directory=") {
			return strings.TrimPrefix(arg, "--target-directory=")
		}
	}
	if operands := nonFlags(args); len(operands) > 1 {
		return operands[len(operands)-1]
	}
	return ""
}

// scratchFile is a plain path in a temporary directory, or a discard
// device, as the redirections the console strips.
var scratchFile = regexp.MustCompile(`^(?:/(?:var/)?tmp/[A-Za-z0-9._+@%,=:/-]*|/dev/(?:null|stdout|stderr|tty|fd/[0-9]+))$`)

func scratchOrDiscard(path string) bool {
	return scratchFile.MatchString(path) && scratchPath(path)
}

// unresolved reports a variable operand (not HOME, which outside judges)
// in a string that names a path outside the workspace anywhere, a loop's
// list or an assignment's value among them: for d in /etc; do rm -rf $d.
// Without such a path it is taken for the workspace's, as before.
func (j *scriptJudge) unresolved(path string) bool {
	if !strings.HasPrefix(path, "$") {
		return false
	}
	for _, named := range j.paths {
		if j.outside(named) {
			return true
		}
	}
	return false
}

// isRoot reports a path that is the workspace itself: removing or moving
// it takes the whole workspace.
func (j *scriptJudge) isRoot(path string) bool {
	if strings.HasPrefix(path, "$") || strings.HasPrefix(path, "~") || j.lost && !strings.HasPrefix(path, "/") {
		return false
	}
	root := filepath.Clean(string(j.workspace))
	if !strings.HasPrefix(path, "/") && !filepath.IsAbs(path) {
		if filepath.Clean(path) == "." {
			return false // rm refuses . and grep -r "rm -rf" . removes nothing
		}
		path = filepath.Join(root, j.cwd, path)
	}
	return root != "." && root != "" && filepath.Clean(path) == root
}

// removesFromInput reports an xargs whose command deletes or writes.
func (j *scriptJudge) removesFromInput(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "-I" || arg == "-n" || arg == "-P" || arg == "-L" || arg == "-d" || arg == "-E" || arg == "-s" || arg == "-a":
			i++ // the option's value
		case strings.HasPrefix(arg, "-"):
		default:
			base := filepath.Base(arg)
			return deleters[base] || writers[base]
		}
	}
	return false
}

// findDeletes reports a find that deletes what it finds.
func findDeletes(args []string) bool {
	for i, arg := range args {
		if arg == "-delete" {
			return true
		}
		if (arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir") && i+1 < len(args) {
			if base := filepath.Base(args[i+1]); deleters[base] || writers[base] {
				return true
			}
		}
	}
	return false
}

// chdir follows a cd: into the workspace, out of it, or somewhere the
// string does not say (home, the previous directory, a variable).
func (j *scriptJudge) chdir(args []string) {
	target := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			target = arg
			break
		}
	}
	switch {
	case target == "" || target == "-" || strings.HasPrefix(target, "~") || strings.HasPrefix(target, "$"):
		j.lost = true
	case strings.HasPrefix(target, "/"):
		rel, err := filepath.Rel(filepath.Clean(string(j.workspace)), filepath.Clean(target))
		if err != nil {
			j.lost = true
			return
		}
		j.cwd, j.lost = rel, false
	case !j.lost:
		j.cwd = filepath.Join(j.cwd, target)
	}
}

// outside reports a path a command deletes or writes outside the
// workspace. A variable other than HOME is not resolved, as before.
func (j *scriptJudge) outside(path string) bool {
	switch {
	case path == "":
		return false
	case strings.HasPrefix(path, "$HOME") || strings.HasPrefix(path, "${HOME"):
		return true
	case strings.HasPrefix(path, "$"):
		return false
	case j.lost && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "~"):
		return true
	}
	return outsideWorkspace(j.workspace, j.cwd, path)
}

// assignment is a NAME=value word before a command.
func assignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	return eq > 0 && !strings.ContainsAny(word[:eq], "/-.$\"'")
}

// unquote drops the quotes that open or close a word.
func unquote(word string) string {
	return strings.Trim(word, "\"'")
}
