package repairbuild

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

// hostEnv is the restricted environment plus the Go settings the console
// passes, so the test builds with the host's caches and offline.
func hostEnv() []string {
	if runtime.GOOS == "windows" {
		// Go needs the profile and system variables there; the console
		// passes them through repairBuilder.
		return append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-p=2")
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GOTOOLCHAIN=local", "GOFLAGS=-p=2"}
	for _, name := range []string{"GOCACHE", "GOPATH", "GOMODCACHE"} {
		if value := os.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// fakeClone is a repository laid out as AXLR: a tui module whose console
// prints the version the linker set.
func fakeClone(t *testing.T, main string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	clone := filepath.Join(t.TempDir(), "20261009-1200-edit-fails")
	write(t, filepath.Join(clone, "tui", "go.mod"), "module github.com/underpass-ai/AXLR\n\ngo 1.21\n")
	write(t, filepath.Join(clone, "tui", "buildinfo", "version.go"), "package buildinfo\n\nvar Version = \"0.0.0-dev\"\n")
	write(t, filepath.Join(clone, "tui", "cmd", "axlr-tui", "main.go"), main)
	git(t, clone, "init", "-q")
	git(t, clone, "add", ".")
	git(t, clone, "commit", "-qm", "repair")
	return clone
}

const printVersion = "package main\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/underpass-ai/AXLR/buildinfo\"\n)\n\nfunc main() { fmt.Print(buildinfo.Version) }\n"

func TestBuildCompilesTheCloneWithTheRepairVersion(t *testing.T) {
	clone := fakeClone(t, printVersion)
	candidate, err := (Builder{Env: hostEnv()}).Build(context.Background(), clone, "20261009-1200-edit-fails")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Path != CandidatePath(clone) || len(candidate.Revision) != 12 || candidate.Version != "repair-20261009-1200-edit-fails-"+candidate.Revision {
		t.Fatalf("candidate = %+v", candidate)
	}
	out, err := exec.Command(candidate.Path).Output()
	if err != nil || string(out) != candidate.Version {
		t.Fatalf("candidate reports %q (%v), want %q", out, err, candidate.Version)
	}
	status, err := exec.Command("git", "-C", clone, "status", "--porcelain").Output()
	if err != nil || len(status) != 0 {
		t.Fatalf("the build changed the clone: %q %v", status, err)
	}
}

func TestBuildReportsFailuresAndClonesWithoutAConsole(t *testing.T) {
	clone := fakeClone(t, "package main\n\nfunc main() { undefinedCall() }\n")
	_, err := (Builder{Env: hostEnv()}).Build(context.Background(), clone, "x")
	if err == nil || !strings.Contains(err.Error(), "undefinedCall") {
		t.Fatalf("build error = %v", err)
	}
	if _, err := (Builder{Env: hostEnv()}).Build(context.Background(), t.TempDir(), "x"); !errors.Is(err, application.ErrNoCandidate) {
		t.Fatalf("plain clone = %v", err)
	}
	if _, err := (Builder{Env: []string{"PATH=/nonexistent"}}).Build(context.Background(), clone, "x"); !errors.Is(err, application.ErrNoToolchain) || !strings.Contains(err.Error(), "not on the console's PATH") {
		t.Fatalf("no toolchain = %v", err)
	}
}

func TestInstallKeepsThePreviousConsoleAndRefusesTwice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("installing is refused on Windows")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "axlr-tui")
	write(t, target, "old build")
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "axlr-tui")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate")
	write(t, candidate, "new build")
	builder := Builder{Executable: func() (string, error) { return link, nil }}
	installed, err := builder.Install(context.Background(), application.RepairCandidate{Path: candidate}, "slug")
	if err != nil {
		t.Fatal(err)
	}
	if installed.Path != target || installed.Backup != target+".before-repair-slug" {
		t.Fatalf("installed = %+v", installed)
	}
	if data, _ := os.ReadFile(target); string(data) != "new build" {
		t.Fatalf("target = %q", data)
	}
	if data, _ := os.ReadFile(installed.Backup); string(data) != "old build" {
		t.Fatalf("backup = %q", data)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v %v", info, err)
	}
	if _, err := builder.Install(context.Background(), application.RepairCandidate{Path: candidate}, "slug"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second install = %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".axlr-tui.repair-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
	if _, err := (Builder{GOOS: "windows"}).Install(context.Background(), application.RepairCandidate{Path: candidate}, "s"); err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("windows = %v", err)
	}
}
