package diagnostics

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// AppLogName is the console's readable log in the default diagnostics
// directory; every console of the account appends to it, and each line
// names the console's process ID.
const AppLogName = "axlr.log"

// appLogMaxBytes is the size at which the log moves to AppLogName+".1",
// replacing the previous one, so the two files hold at most twice this.
const appLogMaxBytes = 4 << 20

// appLogLineBytes caps one entry; a longer message is cut with a marker.
const appLogLineBytes = 2048

// AppLevel is the severity of one app log entry.
type AppLevel string

const (
	AppInfo  AppLevel = "INFO"
	AppWarn  AppLevel = "WARN"
	AppError AppLevel = "ERROR"
)

// Rank orders levels for filtering; an unknown level ranks as INFO.
func (level AppLevel) Rank() int {
	switch level {
	case AppWarn:
		return 1
	case AppError:
		return 2
	default:
		return 0
	}
}

// AppLog is the console's readable log: startup notes, plugin connections
// and the standard error of the plugins it launches, tool failures. Unlike
// the trace it holds text (error messages, plugin names, paths), so every
// entry passes through the redactor first; it never records prompts, model
// output or tool results. Writes are best effort: a failure never reaches
// the console.
type AppLog struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	max     int64
	secrets []string
	pid     int
	now     func() time.Time
}

// OpenAppLog appends to AppLogName in dir, created with owner-only
// permissions. secrets are replaced by [redacted] in every entry.
func OpenAppLog(dir string, secrets ...string) (*AppLog, error) {
	log := &AppLog{path: filepath.Join(dir, AppLogName), max: appLogMaxBytes, pid: os.Getpid(), now: time.Now}
	log.AddSecrets(secrets...)
	if err := log.reopen(); err != nil {
		return nil, err
	}
	return log, nil
}

// Path is the log's file.
func (l *AppLog) Path() string { return l.path }

// PID is the process ID this console's entries carry.
func (l *AppLog) PID() int { return l.pid }

// AddSecrets adds values to redact; short values are ignored, since
// replacing them would garble ordinary text.
func (l *AppLog) AddSecrets(secrets ...string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); len(secret) >= 8 {
			l.secrets = append(l.secrets, secret)
		}
	}
}

// Logf appends one entry. Line breaks in the message become " | ".
func (l *AppLog) Logf(level AppLevel, format string, args ...any) {
	if l == nil {
		return
	}
	l.write(level, fmt.Sprintf(format, args...))
}

func (l *AppLog) write(level AppLevel, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	message = redactAppLog(message, l.secrets)
	message = strings.ReplaceAll(strings.TrimRight(message, "\r\n"), "\r", "")
	message = strings.ReplaceAll(message, "\n", " | ")
	if len(message) > appLogLineBytes {
		message = strings.ToValidUTF8(message[:appLogLineBytes], "") + " [cut]"
	}
	line := fmt.Sprintf("%s %-5s [%d] %s\n", l.now().Format("2006-01-02T15:04:05.000Z07:00"), level, l.pid, message)
	l.rotate()
	if l.file != nil {
		_, _ = l.file.WriteString(line)
	}
}

// rotate follows another console's rotation and rotates a full log. Two
// consoles rotating at once may lose one generation; entries are never
// written to a file that is not the log or its previous generation.
func (l *AppLog) rotate() {
	current, err := os.Lstat(l.path)
	open, openErr := l.file.Stat()
	if err != nil || openErr != nil || !os.SameFile(current, open) {
		_ = l.reopen()
		return
	}
	if current.Size() < l.max {
		return
	}
	// Windows cannot rename an open file; the log then keeps growing.
	if os.Rename(l.path, l.path+".1") == nil {
		_ = l.reopen()
	}
}

func (l *AppLog) reopen() error {
	file, err := openDiagnosticFile(l.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open app log: %w", err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("app log is not a regular file")
	}
	if err == nil {
		err = file.Chmod(0o600)
	}
	if err != nil {
		file.Close()
		return fmt.Errorf("secure app log: %w", err)
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	l.file = file
	return nil
}

// urlCredential is a credential carried in a URL's query, such as the
// access key of a local viewer a plugin prints on stderr.
var urlCredential = regexp.MustCompile(`(?i)([?&](?:k|key|token|access_token|api_key|apikey|secret|sig|signature|password|auth|code)=)[^&#\s"'<>]+`)

// redactAppLog removes the configured secrets, the credentials payload
// captures recognise and credentials in URL queries.
func redactAppLog(message string, secrets []string) string {
	message = string((&PayloadRecorder{secrets: secrets}).redact([]byte(message)))
	return urlCredential.ReplaceAllString(message, "${1}[redacted]")
}

// Writer logs each line written to it as one entry at level, after prefix.
// A partial line waits for its end or for 4 KiB.
func (l *AppLog) Writer(level AppLevel, prefix string) io.Writer {
	return &appLogWriter{log: l, level: level, prefix: prefix}
}

type appLogWriter struct {
	mu      sync.Mutex
	log     *AppLog
	level   AppLevel
	prefix  string
	pending []byte
}

func (w *appLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			if len(w.pending) < 4096 {
				return len(p), nil
			}
			end = len(w.pending)
		}
		if line := strings.TrimSpace(string(w.pending[:min(end, len(w.pending))])); line != "" && !bannerArt(line) && w.log != nil {
			w.log.write(w.level, w.prefix+line)
		}
		w.pending = w.pending[min(end+1, len(w.pending)):]
		if len(w.pending) == 0 {
			w.pending = nil
			return len(p), nil
		}
	}
}

// bannerArt reports a line drawn with box-drawing or block characters, such
// as the KMP logo a plugin prints on standard error at every start. On 10
// October 2026 each console start wrote eight such lines to the log, which
// buried the one line that said the plugin connected.
func bannerArt(line string) bool {
	r, _ := utf8.DecodeRuneInString(line)
	return r >= 0x2500 && r <= 0x259F
}

// Close closes the log; later entries are dropped.
func (l *AppLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// AppLogEntry is one parsed line of the app log.
type AppLogEntry struct {
	Line  string
	Level AppLevel
	PID   int
}

// appLogReadBytes bounds what a tail reads from each generation.
const appLogReadBytes = 1 << 20

// TailAppLog returns the last entries of the log at path, its previous
// generation first, that keep reports true for, at most limit of them, and
// how many more matched before those. A missing file reads as empty.
func TailAppLog(path string, limit int, keep func(AppLogEntry) bool) ([]AppLogEntry, int, error) {
	var matched []AppLogEntry
	for _, name := range []string{path + ".1", path} {
		entries, err := readAppLogEntries(name)
		if err != nil {
			return nil, 0, err
		}
		for _, entry := range entries {
			if keep == nil || keep(entry) {
				matched = append(matched, entry)
			}
		}
	}
	omitted := 0
	if limit >= 0 && len(matched) > limit {
		omitted = len(matched) - limit
		matched = matched[omitted:]
	}
	return matched, omitted, nil
}

func readAppLogEntries(path string) ([]AppLogEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("app log is not a regular file")
	}
	start := max(info.Size()-appLogReadBytes, 0)
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(io.LimitReader(file, appLogReadBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	var entries []AppLogEntry
	first := start > 0
	for scanner.Scan() {
		if first {
			// The read began inside a line.
			first = false
			continue
		}
		if line := scanner.Text(); line != "" {
			entries = append(entries, parseAppLogLine(line))
		}
	}
	return entries, scanner.Err()
}

// parseAppLogLine reads "<time> <LEVEL> [<pid>] <message>"; a line that
// does not match keeps level INFO and PID 0.
func parseAppLogLine(line string) AppLogEntry {
	entry := AppLogEntry{Line: line, Level: AppInfo}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return entry
	}
	switch level := AppLevel(fields[1]); level {
	case AppInfo, AppWarn, AppError:
		entry.Level = level
	}
	if pid, err := strconv.Atoi(strings.Trim(fields[2], "[]")); err == nil && strings.HasPrefix(fields[2], "[") {
		entry.PID = pid
	}
	return entry
}
