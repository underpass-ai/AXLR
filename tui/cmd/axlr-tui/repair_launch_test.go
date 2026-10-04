package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestRepairSlugKeepsTheBriefsWordsAndTheTimestamp(t *testing.T) {
	now := time.Date(2026, 10, 5, 1, 2, 0, 0, time.UTC)
	if got := repairSlug("go test ./... fails: TestWordCount expects 2, gets 3", now); got != "20261005-0102-go-test-fails-testwordcount-expects-2-gets-3" {
		t.Fatalf("slug %q", got)
	}
	if got := repairSlug("¿¡!?", now); got != "20261005-0102-failure" {
		t.Fatalf("empty words %q", got)
	}
	long := repairSlug(strings.Repeat("palabra ", 20), now)
	if len(long) > len("20261005-0102-")+41 {
		t.Fatalf("slug too long: %q", long)
	}
}

// stubTools puts fake gh and git executables first on PATH and records their
// argument lines.
func stubTools(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$(basename \"$0\") $*\" >> " + log + "\n" +
		"case \"$1 $2\" in\n" +
		"  'repo clone') mkdir -p \"$4/.git/info\"; exit 0;;\n" +
		"  'rev-parse --abbrev-ref') echo main;;\n" +
		"  'issue view') printf 'Broken thing\\n\\nIt breaks.\\n\\nhttps://example.test/issues/7\\n';;\n" +
		"esac\nexit 0\n"
	for _, name := range []string{"gh", "git"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir, log
}

func TestPrepareRepairCloneWritesTheMarkerAndReadsIssues(t *testing.T) {
	bin, log := stubTools(t)
	repairs := filepath.Join(t.TempDir(), "repairs")
	env := []string{"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin:/bin", "HOME=" + t.TempDir()}
	settings := storage.RepairSettings{Repository: "o/r", About: "project:r"}
	clone, brief, err := prepareRepairClone(context.Background(), settings, repairs, "#7", env, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(clone, repairs) || !strings.HasPrefix(brief, "Issue #7 of o/r: Broken thing") {
		t.Fatalf("clone %q brief %q", clone, brief)
	}
	data, err := os.ReadFile(filepath.Join(clone, application.RepairMarker))
	if err != nil {
		t.Fatal(err)
	}
	var marker application.RepairMarkerFile
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.Version != 1 || marker.Repository != "o/r" || marker.Base != "main" || marker.Issue != "7" || marker.About != "project:r" || marker.Slug != filepath.Base(clone) {
		t.Fatalf("marker %+v", marker)
	}
	exclude, _ := os.ReadFile(filepath.Join(clone, ".git", "info", "exclude"))
	if !strings.Contains(string(exclude), application.RepairMarker) {
		t.Fatal("the marker must be excluded from the repair commit")
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "gh issue view 7 --repo o/r") || !strings.Contains(string(calls), "gh repo clone o/r "+clone) {
		t.Fatalf("calls:\n%s", calls)
	}
	if _, _, err := prepareRepairClone(context.Background(), settings, repairs, "   ", env, io.Discard); err == nil {
		t.Fatal("an empty brief must be refused")
	}
}

func TestPrepareRepairCloneDefaultsTheAboutAndHonoursTheDirectory(t *testing.T) {
	bin, _ := stubTools(t)
	directory := filepath.Join(t.TempDir(), "elsewhere")
	env := []string{"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin:/bin", "HOME=" + t.TempDir()}
	clone, brief, err := prepareRepairClone(context.Background(), storage.RepairSettings{Repository: "underpass-ai/AXLR", Directory: directory}, "/ignored", "the card hides stdin", env, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(clone) != directory || brief != "the card hides stdin" {
		t.Fatalf("clone %q brief %q", clone, brief)
	}
	data, _ := os.ReadFile(filepath.Join(clone, application.RepairMarker))
	if !strings.Contains(string(data), `"about": "project:axlr"`) {
		t.Fatalf("marker %s", data)
	}
}

func TestPrepareRepairCloneReportsAFailingClone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'gh: not logged in' >&2\nexit 4\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin:/bin", "HOME=" + t.TempDir()}
	_, _, err := prepareRepairClone(context.Background(), storage.RepairSettings{Repository: "o/r"}, t.TempDir(), "x", env, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("got %v", err)
	}
}
