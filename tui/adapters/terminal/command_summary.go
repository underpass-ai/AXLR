package terminal

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

// execDefaultTimeoutMS is local_exec's timeout when the call names none; a
// row mentions the timeout only when the call asked for another.
const execDefaultTimeoutMS = 30000

// formatCommand writes a program and its arguments as a shell would read
// them: an argument with spaces, quotes, shell characters or nothing in it
// is quoted, so `go test ./...` and `git commit -m "a b"` read as typed.
func formatCommand(program string, args []string) string {
	words := make([]string, 0, len(args)+1)
	for _, word := range append([]string{program}, args...) {
		words = append(words, shellWord(word))
	}
	return strings.TrimSpace(strings.Join(words, " "))
}

var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellWord(word string) string {
	if plainShellWord.MatchString(word) {
		return word
	}
	return strconv.Quote(word)
}

// execCommand is a local_exec call's command line: the program and its
// arguments, then the working directory when it is not the workspace root
// and the timeout when it is not the default.
func execCommand(arguments []byte) (string, bool) {
	var call struct {
		Program   string   `json:"program"`
		Args      []string `json:"args"`
		Cwd       string   `json:"cwd"`
		TimeoutMS int64    `json:"timeout_ms"`
	}
	if json.Unmarshal(arguments, &call) != nil || strings.TrimSpace(call.Program) == "" {
		return "", false
	}
	command := singleLine(formatCommand(call.Program, call.Args))
	if cwd := strings.TrimSpace(call.Cwd); cwd != "" && cwd != "." && cwd != "./" {
		command += " (cwd " + singleLine(cwd) + ")"
	}
	if call.TimeoutMS > 0 && call.TimeoutMS != execDefaultTimeoutMS {
		command += " (timeout " + formatDuration(call.TimeoutMS) + ")"
	}
	return command, true
}

// localExecSummary reads a local_exec row as the command it ran. On 10
// October 2026 `go vet ./buildinfo/...` read "go timeout_ms=120000": the
// scalar summary kept the timeout and dropped the arguments, so go test,
// go vet and go run all looked the same.
func localExecSummary(call root.ToolCall) (string, bool) {
	if call.Name != "local_exec" {
		return "", false
	}
	command, ok := execCommand(call.Arguments.Bytes())
	if !ok {
		return "", false
	}
	return ansi.Truncate(command, toolRowArgumentWidth, "…"), true
}
