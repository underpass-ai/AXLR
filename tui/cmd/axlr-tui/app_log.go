package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/application"
)

// logsCommand is the first argument of `axlr-tui logs`, which prints the
// end of the app log without starting a console.
const logsCommand = "logs"

// openAppLog opens the app log in the default diagnostics directory. The
// log helps after the fact and must not stop a launch: an error leaves the
// console without one, which the caller says on stderr.
func openAppLog(getenv func(string) string, secrets ...string) (*diagnostics.AppLog, error) {
	dir, err := diagnostics.DefaultDirectory(getenv)
	if err != nil {
		return nil, err
	}
	return diagnostics.OpenAppLog(dir, secrets...)
}

// warningWords mark a console note on stderr that reports trouble.
var warningWords = regexp.MustCompile(`(?i)\b(ignoring|unavailable|refused|incomplete|needs|unsandboxed|invalid|failed|error)\b`)

// consoleNotes copies each line the console writes on stderr to the app
// log: the notes before the terminal opens are hidden once it does.
type consoleNotes struct {
	out     io.Writer
	info    io.Writer
	warning io.Writer
}

func teeConsoleNotes(out io.Writer, log *diagnostics.AppLog) io.Writer {
	if log == nil {
		return out
	}
	return consoleNotes{out: out, info: log.Writer(diagnostics.AppInfo, ""), warning: log.Writer(diagnostics.AppWarn, "")}
}

func (c consoleNotes) Write(p []byte) (int, error) {
	if warningWords.Match(p) {
		_, _ = c.warning.Write(p)
	} else {
		_, _ = c.info.Write(p)
	}
	return c.out.Write(p)
}

// pluginLog records the plugin manager's connections and the standard
// error of the servers it launches, each named by its plugin ID.
type pluginLog struct {
	log *diagnostics.AppLog
}

func (p pluginLog) PluginConnection(id root.PluginID, event plugins.ConnectionEvent, shared bool, err error) {
	switch {
	case event == plugins.Connected && shared:
		p.log.Logf(diagnostics.AppInfo, "plugin %s: connected to the shared engine", id)
	case event == plugins.Connected:
		p.log.Logf(diagnostics.AppInfo, "plugin %s: connected", id)
	case event == plugins.ConnectFailed:
		p.log.Logf(diagnostics.AppWarn, "plugin %s: connection failed, the next call tries again: %v", id, err)
	case event == plugins.ConnectionLost:
		p.log.Logf(diagnostics.AppWarn, "plugin %s: connection lost, the next call starts it again: %v", id, err)
	}
}

func (p pluginLog) PluginStderr(id root.PluginID) io.Writer {
	return p.log.Writer(diagnostics.AppInfo, "plugin "+id.String()+" stderr: ")
}

// pluginSecretKey names a plugin environment variable whose value the app
// log redacts, since a plugin may print its environment on stderr.
var pluginSecretKey = regexp.MustCompile(`(?i)(key|token|secret|pass|auth|credential|cookie)`)

func pluginSecrets(registrations []plugins.Registration) []string {
	var secrets []string
	for _, registration := range registrations {
		for _, entry := range registration.Env {
			if name, value, ok := strings.Cut(entry, "="); ok && pluginSecretKey.MatchString(name) {
				secrets = append(secrets, value)
			}
		}
	}
	return secrets
}

// appLogReader serves axlr_logs from the app log.
type appLogReader struct {
	log *diagnostics.AppLog
}

func (r appLogReader) TailLog(_ context.Context, query application.AppLogQuery) (application.AppLogPage, error) {
	entries, omitted, err := diagnostics.TailAppLog(r.log.Path(), query.Lines, logFilter(diagnostics.AppLevel(query.Level), query.Contains, query.AllConsoles, r.log.PID()))
	if err != nil {
		return application.AppLogPage{}, err
	}
	page := application.AppLogPage{Path: r.log.Path(), Console: r.log.PID(), Entries: make([]string, 0, len(entries)), Omitted: omitted}
	for _, entry := range entries {
		page.Entries = append(page.Entries, entry.Line)
	}
	return page, nil
}

func logFilter(level diagnostics.AppLevel, contains string, all bool, pid int) func(diagnostics.AppLogEntry) bool {
	contains = strings.ToLower(contains)
	return func(entry diagnostics.AppLogEntry) bool {
		return entry.Level.Rank() >= level.Rank() && (all || entry.PID == pid) && (contains == "" || strings.Contains(strings.ToLower(entry.Line), contains))
	}
}

// runLogs prints the end of the app log: `axlr-tui logs [--lines N]
// [--level WARN] [--contains TEXT] [--pid PID]`, every console by default.
func runLogs(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("axlr-tui logs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lines := flags.Int("lines", 50, "entries to print, the newest last")
	level := flags.String("level", "INFO", "lowest level to print: INFO, WARN or ERROR")
	contains := flags.String("contains", "", "print only entries containing this text")
	pid := flags.Int("pid", 0, "print only the entries of the console with this process ID")
	path := flags.Bool("path", false, "print the log's path and nothing else")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *lines < 1 {
		fmt.Fprintln(stderr, "axlr-tui logs: takes --lines (1 or more), --level, --contains, --pid and --path")
		return 2
	}
	upper := diagnostics.AppLevel(strings.ToUpper(*level))
	if upper != diagnostics.AppInfo && upper != diagnostics.AppWarn && upper != diagnostics.AppError {
		fmt.Fprintln(stderr, "axlr-tui logs: --level is INFO, WARN or ERROR")
		return 2
	}
	dir, err := diagnostics.DefaultDirectory(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui logs:", err)
		return 1
	}
	file := filepath.Join(dir, diagnostics.AppLogName)
	if *path {
		fmt.Fprintln(stdout, file)
		return 0
	}
	filter := logFilter(upper, *contains, *pid == 0, *pid)
	entries, omitted, err := diagnostics.TailAppLog(file, *lines, filter)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui logs:", err)
		return 1
	}
	if omitted > 0 {
		fmt.Fprintln(stderr, "axlr-tui logs: "+strconv.Itoa(omitted)+" earlier entries omitted; raise --lines to see them")
	}
	for _, entry := range entries {
		fmt.Fprintln(stdout, entry.Line)
	}
	return 0
}

// logReader is axlr_logs' port, nil without an app log so the tool is not
// offered.
func logReader(log *diagnostics.AppLog) application.AppLogPort {
	if log == nil {
		return nil
	}
	return appLogReader{log: log}
}

// failureLogger records the tool calls that failed inside AXLR, nil
// without an app log.
func failureLogger(log *diagnostics.AppLog) func(string) {
	if log == nil {
		return nil
	}
	return func(failure string) { log.Logf(diagnostics.AppWarn, "%s", failure) }
}
