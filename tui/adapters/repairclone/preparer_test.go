package repairclone

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

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

func TestPreparerClonesWritesTheMarkerWithItsOriginAndExcludesIt(t *testing.T) {
	bin, log := stubTools(t)
	repairs := filepath.Join(t.TempDir(), "repairs")
	preparer := Preparer{Repairs: repairs, Env: []string{"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin:/bin", "HOME=" + t.TempDir()}}
	clone, err := preparer.Prepare(context.Background(), application.RepairCloneRequest{Repository: "o/r", Brief: "local_edit fails", Slug: "20261005-1200-local-edit-fails", Origin: "0123456789abcdef0123456789abcdef", Build: "0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if clone.Path != filepath.Join(repairs, "20261005-1200-local-edit-fails") || clone.Base != "main" {
		t.Fatalf("clone %+v", clone)
	}
	data, err := os.ReadFile(filepath.Join(clone.Path, application.RepairMarker))
	if err != nil {
		t.Fatal(err)
	}
	var marker application.RepairMarkerFile
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.Version != 1 || marker.Repository != "o/r" || marker.About != "project:r" || marker.Origin != "0123456789abcdef0123456789abcdef" || marker.Build != "0.3.0" || marker.Brief != "local_edit fails" {
		t.Fatalf("marker %+v", marker)
	}
	exclude, _ := os.ReadFile(filepath.Join(clone.Path, ".git", "info", "exclude"))
	if !strings.Contains(string(exclude), application.RepairMarker) {
		t.Fatal("the marker must be excluded from the repair commit")
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "gh repo clone o/r "+clone.Path) {
		t.Fatalf("calls:\n%s", calls)
	}
	if _, err := preparer.Prepare(context.Background(), application.RepairCloneRequest{Repository: "o/r", Slug: "20261005-1200-local-edit-fails"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second clone of the same slug: %v", err)
	}
	issue, err := preparer.Issue(context.Background(), "o/r", "7")
	if err != nil || !strings.HasPrefix(issue, "Issue #7 of o/r: Broken thing") {
		t.Fatalf("issue %q %v", issue, err)
	}
}

func TestPreparerRefusesBadRequestsAndReportsAFailingClone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs")
	}
	preparer := Preparer{Repairs: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}
	for name, request := range map[string]application.RepairCloneRequest{
		"repository": {Slug: "x"},
		"slug":       {Repository: "o/r"},
		"path slug":  {Repository: "o/r", Slug: "../x"},
		"dot slug":   {Repository: "o/r", Slug: ".hidden"},
	} {
		if _, err := preparer.Prepare(context.Background(), request); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := (Preparer{Repairs: "relative"}).Prepare(context.Background(), application.RepairCloneRequest{Repository: "o/r", Slug: "x"}); err == nil {
		t.Fatal("relative repairs directory accepted")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'gh: not logged in' >&2\nexit 4\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	failing := Preparer{Repairs: t.TempDir(), Env: []string{"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin:/bin"}}
	_, err := failing.Prepare(context.Background(), application.RepairCloneRequest{Repository: "o/r", Slug: "x"})
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("got %v", err)
	}
	if _, err := LookPath([]string{"PATH=" + bin}, "gh"); err != nil {
		t.Fatal(err)
	}
	if _, err := LookPath([]string{"PATH=" + bin}, "missing"); err == nil {
		t.Fatal("missing program found")
	}
}
