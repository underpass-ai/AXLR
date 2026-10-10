//go:build linux

package local

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		if appArmorRestrictsUserNamespaces() {
			return fmt.Errorf("bwrap cannot create a sandbox here: %s (%v %s)", appArmorUserNamespaces, err, detail)
		}
		return fmt.Errorf("bwrap cannot create a sandbox here: %v %s", err, detail)
	}
	return nil
}

// appArmorUserNamespaces names the cause and the fix when Ubuntu's AppArmor
// refuses bwrap. On 10 October 2026 the console QA ran on such a machine
// and the probe reported only bwrap's symptom ("loopback: Failed
// RTM_NEWADDR: Operation not permitted", or "setting up uid map: Permission
// denied" with the network on).
const appArmorUserNamespaces = "Ubuntu's AppArmor blocks unprivileged user namespaces (kernel.apparmor_restrict_unprivileged_userns=1); add an AppArmor profile for /usr/bin/bwrap or set that sysctl to 0"

// readAppArmorUserNamespaces reads the sysctl that makes AppArmor refuse
// unprivileged user namespaces; a test replaces it.
var readAppArmorUserNamespaces = func() ([]byte, error) {
	return os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
}

// appArmorRestrictsUserNamespaces reports whether that sysctl reads 1.
func appArmorRestrictsUserNamespaces() bool {
	value, err := readAppArmorUserNamespaces()
	return err == nil && strings.TrimSpace(string(value)) == "1"
}
