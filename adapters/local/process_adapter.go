package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

type ProcessAdapter struct {
	Root string
	Env  []string
}

func (a *ProcessAdapter) Run(ctx context.Context, c domain.ExecCommand) (domain.ExecResult, error) {
	cwd, err := a.workspaceCwd(string(c.Cwd))
	if err != nil {
		return domain.ExecResult{}, err
	}
	if ctx.Err() != nil {
		return domain.ExecResult{}, &domain.Fault{Status: "cancelled", Code: "cancelled", Message: "request cancelled before process start"}
	}
	program, err := a.resolveProgram(string(c.Program), cwd)
	if err != nil {
		return domain.ExecResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout))
	defer cancel()
	cmd := exec.CommandContext(runCtx, program, []string(c.Args)...)
	cmd.Dir = cwd
	cmd.Env = append([]string{}, a.Env...)
	cmd.Stdin = strings.NewReader(string(c.Stdin))
	capture := &capture{budget: int(c.OutputLimit)}
	cmd.Stdout = streamWriter{c: capture}
	cmd.Stderr = streamWriter{c: capture, stderr: true}
	configureProcess(cmd)
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return domain.ExecResult{}, domain.Fail("start_failed", err.Error())
	}
	err = cmd.Wait()
	if errors.Is(err, exec.ErrWaitDelay) {
		_ = killProcessTree(cmd)
	}
	if ctx.Err() != nil {
		return domain.ExecResult{}, &domain.Fault{Status: "cancelled", Code: "cancelled", Message: "process cancelled; prior effects may remain"}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return domain.ExecResult{}, &domain.Fault{Status: "timed_out", Code: "timeout", Message: "process timed out; prior effects may remain"}
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return domain.ExecResult{}, domain.Fail("process_error", err.Error())
	}
	code := 0
	if exitErr != nil {
		code = exitErr.ExitCode()
	}
	return domain.ExecResult{ExitCode: code, Stdout: strings.ToValidUTF8(capture.stdout.String(), "�"), Stderr: strings.ToValidUTF8(capture.stderr.String(), "�"), CapturedBytes: capture.captured, DiscardedBytes: capture.discarded, Truncated: capture.discarded > 0}, nil
}
func (a *ProcessAdapter) workspaceCwd(p string) (string, error) {
	if p == "" {
		p = "."
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(a.Root, p))
	if err != nil {
		return "", fileError(err)
	}
	rel, err := filepath.Rel(a.Root, resolved)
	if err != nil {
		return "", fileError(err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", domain.Reject("invalid_path", "cwd escapes workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fileError(err)
	}
	if !info.IsDir() {
		return "", domain.Reject("invalid_path", "cwd is not a directory")
	}
	return resolved, nil
}
func (a *ProcessAdapter) resolveProgram(program, cwd string) (string, error) {
	if filepath.IsAbs(program) {
		return program, nil
	}
	if strings.ContainsRune(program, filepath.Separator) {
		return filepath.Join(cwd, program), nil
	}
	pathValue := ""
	for _, v := range a.Env {
		if pathEnvironment(v) {
			pathValue = v[len("PATH="):]
		}
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if !filepath.IsAbs(dir) {
			continue
		}
		for _, candidate := range programCandidates(filepath.Join(dir, program)) {
			info, err := os.Stat(candidate)
			if err == nil && executableFile(candidate, info) {
				return candidate, nil
			}
		}
	}
	return "", domain.Fail("start_failed", fmt.Sprintf("program %q not found in configured PATH", program))
}
