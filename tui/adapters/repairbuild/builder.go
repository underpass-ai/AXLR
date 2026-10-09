// Package repairbuild builds the repaired axlr-tui from a repair clone and
// installs it over the running console on request.
package repairbuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// VersionVariable is the linker variable the release sets to the version.
const VersionVariable = "github.com/underpass-ai/AXLR/buildinfo.Version"

// buildTimeout bounds one build; a cold module cache on a slow link is the
// slow case.
const buildTimeout = 10 * time.Minute

// outputTail bounds the build output a failure quotes.
const outputTail = 4096

// Builder is application.RepairCandidatePort over the Go toolchain. Env is
// the console's restricted environment (PATH and HOME); the build adds
// GOFLAGS=-p=2 so it does not starve local models of memory, unless Env
// sets GOFLAGS. Executable is the running console's path, os.Executable
// when nil; GOOS is runtime.GOOS when empty.
type Builder struct {
	Env        []string
	Executable func() (string, error)
	GOOS       string
}

var _ application.RepairCandidatePort = Builder{}

// CandidatePath is where a clone's candidate is built: beside the clone, so
// the clone's worktree stays as the pull request left it.
func CandidatePath(clone string) string {
	name := filepath.Base(clone) + ".axlr-tui"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(clone), name)
}

// Build compiles tui/cmd/axlr-tui of the clone at its HEAD with the
// version repair-<slug>-<commit>.
func (b Builder) Build(ctx context.Context, clone, slug string) (application.RepairCandidate, error) {
	module := filepath.Join(clone, "tui")
	if info, err := os.Stat(filepath.Join(module, "cmd", "axlr-tui")); err != nil || !info.IsDir() {
		return application.RepairCandidate{}, application.ErrNoCandidate
	}
	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	revision, err := b.run(ctx, clone, "git", "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return application.RepairCandidate{}, fmt.Errorf("read the clone's commit: %w", err)
	}
	revision = strings.TrimSpace(revision)
	version := "repair-" + slug + "-" + revision
	output := CandidatePath(clone)
	if _, err := b.run(ctx, module, "go", "build", "-trimpath", "-ldflags", "-X "+VersionVariable+"="+version, "-o", output, "./cmd/axlr-tui"); err != nil {
		return application.RepairCandidate{}, err
	}
	return application.RepairCandidate{Path: output, Version: version, Revision: revision}, nil
}

// run runs program in dir and returns its stdout; a failure quotes the
// tail of both streams.
func (b Builder) run(ctx context.Context, dir, program string, args ...string) (string, error) {
	path, err := lookPath(b.Env, program, runtime.GOOS)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not on the console's PATH", application.ErrNoToolchain, program)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir, cmd.Env = dir, b.environment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if len(combined) > outputTail {
			combined = "…" + strings.ToValidUTF8(combined[len(combined)-outputTail:], "")
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", fmt.Errorf("%s %s: %v: %s", program, strings.Join(args, " "), err, combined)
	}
	return stdout.String(), nil
}

// lookPath finds program on the PATH entry of env. On Windows the entry may
// be spelled Path, the program ends in .exe and files carry no execute bit.
func lookPath(env []string, program, goos string) (string, error) {
	windows := goos == "windows"
	names := []string{program}
	if windows && filepath.Ext(program) == "" {
		names = append(names, program+".exe")
	}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key != "PATH" && !(windows && strings.EqualFold(key, "PATH")) {
			continue
		}
		for _, dir := range filepath.SplitList(value) {
			for _, name := range names {
				candidate := filepath.Join(dir, name)
				if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && (windows || info.Mode()&0o111 != 0) {
					return candidate, nil
				}
			}
		}
	}
	return "", exec.ErrNotFound
}

func (b Builder) environment() []string {
	env := append([]string(nil), b.Env...)
	for _, entry := range env {
		if strings.HasPrefix(entry, "GOFLAGS=") {
			return env
		}
	}
	return append(env, "GOFLAGS=-p=2")
}

// Install puts the candidate where the running console's executable is,
// through a temporary file in the same directory renamed over it; the
// previous executable is kept, hard-linked, as <name>.before-repair-<slug>. The
// running process keeps its open file, so nothing breaks before the person
// restarts. Windows cannot rename over a running executable and is refused.
func (b Builder) Install(_ context.Context, candidate application.RepairCandidate, slug string) (application.RepairInstallation, error) {
	goos := b.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return application.RepairInstallation{}, errors.New("Windows cannot replace a running executable: copy the candidate over axlr-tui after closing every console")
	}
	executable := b.Executable
	if executable == nil {
		executable = os.Executable
	}
	target, err := executable()
	if err != nil {
		return application.RepairInstallation{}, fmt.Errorf("find the running console: %w", err)
	}
	if target, err = filepath.EvalSymlinks(target); err != nil {
		return application.RepairInstallation{}, fmt.Errorf("find the running console: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return application.RepairInstallation{}, err
	}
	backup := target + ".before-repair-" + slug
	if _, err := os.Lstat(backup); err == nil {
		return application.RepairInstallation{}, fmt.Errorf("%s already exists: this repair was installed before", backup)
	}
	source, err := os.Open(candidate.Path)
	if err != nil {
		return application.RepairInstallation{}, fmt.Errorf("open the candidate: %w", err)
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".repair-*")
	if err != nil {
		return application.RepairInstallation{}, fmt.Errorf("write beside %s: %w", target, err)
	}
	cleanup := func() { _ = os.Remove(temporary.Name()) }
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		cleanup()
		return application.RepairInstallation{}, fmt.Errorf("copy the candidate: %w", err)
	}
	if err := temporary.Chmod(info.Mode().Perm() | 0o100); err != nil {
		temporary.Close()
		cleanup()
		return application.RepairInstallation{}, err
	}
	if err := errors.Join(temporary.Sync(), temporary.Close()); err != nil {
		cleanup()
		return application.RepairInstallation{}, err
	}
	// A hard link keeps the previous console while the rename replaces the
	// path in one step, so a console started meanwhile finds one or the
	// other.
	if err := os.Link(target, backup); err != nil {
		cleanup()
		return application.RepairInstallation{}, fmt.Errorf("keep the previous console: %w", err)
	}
	if err := os.Rename(temporary.Name(), target); err != nil {
		cleanup()
		_ = os.Remove(backup)
		return application.RepairInstallation{}, fmt.Errorf("install the candidate: %w", err)
	}
	return application.RepairInstallation{Path: target, Backup: backup}, nil
}
