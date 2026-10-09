//go:build linux

package local

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// wrap runs program under bubblewrap: the root filesystem bound read-only,
// fresh /dev and /proc, a private /tmp, then the workspace and the writable
// paths bound read-write over it. bwrap stays in the process group the
// adapter kills on cancellation and dies with the console
// (--die-with-parent); the program's exit status is bwrap's.
func (s *Sandbox) wrap(root, cwd, program string, args []string) (string, []string, error) {
	wrapped := []string{"--die-with-parent", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp", "--bind", root, root}
	for _, path := range s.Writable {
		wrapped = append(wrapped, "--bind", path, path)
	}
	if !s.Network {
		wrapped = append(wrapped, "--unshare-net")
	}
	wrapped = append(wrapped, "--chdir", cwd, "--", program)
	return s.Program, append(wrapped, args...), nil
}

// ProbeSandbox runs a trivial command under bwrap with the sandbox's
// options, so a kernel or security policy that refuses it (unprivileged
// user namespaces disabled, an AppArmor rule) is found at launch rather
// than on the model's first command.
func ProbeSandbox(ctx context.Context, s Sandbox) error {
	if s.Program == "" {
		return errors.New("bwrap is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	program, args, err := s.wrap("/", "/", "/bin/true", nil)
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return fmt.Errorf("bwrap cannot create a sandbox here: %v %s", err, detail)
	}
	return nil
}
