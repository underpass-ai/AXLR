package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Each entry is one private line with its time, level and console, its
// secrets redacted and its line breaks folded.
func TestAppLogWritesRedactedPrivateLines(t *testing.T) {
	dir := t.TempDir()
	log, err := OpenAppLog(dir, "sk-or-secret-value", "short")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.now = func() time.Time { return time.Date(2026, 10, 9, 15, 4, 5, 0, time.UTC) }
	log.AddSecrets("typesafe-token-1")
	log.Logf(AppWarn, "plugin kmp: connection failed: key sk-or-secret-value and typesafe-token-1\nsecond line")
	log.Logf(AppInfo, "short stays")
	text := readLog(t, log.Path())
	want := fmt.Sprintf("2026-10-09T15:04:05.000Z WARN  [%d] plugin kmp: connection failed: key [redacted] and [redacted] | second line\n", os.Getpid())
	if !strings.HasPrefix(text, want) || !strings.Contains(text, "INFO  ["+fmt.Sprint(os.Getpid())+"] short stays\n") {
		t.Fatalf("log = %q", text)
	}
	if info, err := os.Stat(log.Path()); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("mode=%v err=%v", info, err)
	}
	if filepath.Base(log.Path()) != AppLogName {
		t.Fatalf("path = %s", log.Path())
	}
}

// A full log moves to .1 and a new one starts; a console that finds the
// log rotated by another follows it.
func TestAppLogRotatesAndFollowsAnotherConsole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot rename an open log")
	}
	dir := t.TempDir()
	first, err := OpenAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	first.max = 200
	for i := 0; i < 5; i++ {
		first.Logf(AppInfo, "entry %d %s", i, strings.Repeat("x", 40))
	}
	previous := readLog(t, first.Path()+".1")
	current := readLog(t, first.Path())
	if !strings.Contains(previous, "entry 0") || strings.Contains(current, "entry 0") || !strings.Contains(current, "entry 4") {
		t.Fatalf("previous=%q current=%q", previous, current)
	}
	second.Logf(AppWarn, "from the second console")
	if !strings.Contains(readLog(t, first.Path()), "from the second console") {
		t.Fatal("second console kept writing to the rotated file")
	}
}

func TestAppLogWriterSplitsLinesAndTailFilters(t *testing.T) {
	dir := t.TempDir()
	log, err := OpenAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	writer := log.Writer(AppInfo, "plugin kmp stderr: ")
	_, _ = writer.Write([]byte("starting\nhalf "))
	_, _ = writer.Write([]byte("done\n\n"))
	log.Logf(AppError, "boom")
	log.Close()
	log.Logf(AppError, "after close is dropped")
	other := fmt.Sprintf("2026-10-09T15:04:05.000Z WARN  [%d] another console\nnot an entry\n", os.Getpid()+1)
	file, err := os.OpenFile(log.Path(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(other)
	file.Close()
	all, omitted, err := TailAppLog(log.Path(), 10, nil)
	if err != nil || omitted != 0 || len(all) != 5 {
		t.Fatalf("all=%+v omitted=%d err=%v", all, omitted, err)
	}
	if !strings.HasSuffix(all[0].Line, "plugin kmp stderr: starting") || !strings.HasSuffix(all[1].Line, "plugin kmp stderr: half done") || all[2].Level != AppError || all[3].PID != os.Getpid()+1 || all[4].Level != AppInfo || all[4].PID != 0 {
		t.Fatalf("entries = %+v", all)
	}
	warnings, omitted, err := TailAppLog(log.Path(), 1, func(e AppLogEntry) bool { return e.Level.Rank() >= AppWarn.Rank() })
	if err != nil || omitted != 1 || len(warnings) != 1 || !strings.HasSuffix(warnings[0].Line, "another console") {
		t.Fatalf("warnings=%+v omitted=%d err=%v", warnings, omitted, err)
	}
	if entries, _, err := TailAppLog(filepath.Join(dir, "absent.log"), 5, nil); err != nil || len(entries) != 0 {
		t.Fatalf("absent log: %+v %v", entries, err)
	}
}

// A tail reads a bounded end of each file and skips the line it began in.
func TestTailAppLogReadsABoundedEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), AppLogName)
	var text strings.Builder
	for text.Len() < appLogReadBytes+4096 {
		fmt.Fprintf(&text, "2026-10-09T15:04:05.000Z INFO  [7] %s\n", strings.Repeat("y", 100))
	}
	text.WriteString("2026-10-09T15:04:05.000Z ERROR [7] last\n")
	if err := os.WriteFile(path, []byte(text.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, _, err := TailAppLog(path, -1, nil)
	if err != nil || len(entries) == 0 || entries[len(entries)-1].Level != AppError {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	for _, entry := range entries {
		if entry.PID != 7 {
			t.Fatalf("partial line kept: %q", entry.Line)
		}
	}
}

func TestAppLogRedactsRecognisedCredentials(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"memory viewer at http://127.0.0.1:34265/?k=ea95d0630bd46cd9", "memory viewer at http://127.0.0.1:34265/?k=[redacted]"},
		{"GET /cb?state=1&code=abc123&x=2", "GET /cb?state=1&code=[redacted]&x=2"},
		{"Authorization: Bearer abc.def-ghi", "Authorization: [redacted]"},
		{"token ghp_" + strings.Repeat("a", 36) + " used", "token [redacted] used"},
		{"keyboard=us mode", "keyboard=us mode"},
	} {
		if got := redactAppLog(tc.in, nil); got != tc.want {
			t.Fatalf("redactAppLog(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNilAppLogIgnoresEntries(t *testing.T) {
	var log *AppLog
	log.Logf(AppWarn, "nothing")
	log.AddSecrets("abcdefghij")
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
}
