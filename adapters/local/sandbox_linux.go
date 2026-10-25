//go:build linux

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
)

// Paths inside an isolated sandbox: the relay and the proxy's socket for an
// allow list, on the private /run.
const (
	sandboxRelay  = "/run/axlr/relay"
	sandboxSocket = "/run/axlr/egress.sock"
)

// launch starts program under the sandbox: in its container, or under
// bubblewrap with the root filesystem bound read-only, fresh /dev and
// /proc, a private /tmp, then the workspace and the writable paths bound
// read-write over it. bwrap stays in the process group the adapter kills
// on cancellation and dies with the console (--die-with-parent); the
// program's exit status is bwrap's.
func (s *Sandbox) launch(root, cwd, program string, args []string) (launch, error) {
	if s.Container != nil {
		return s.Container.launch(s, root, cwd, program, args)
	}
	if !s.Isolate {
		wrapped := []string{"--die-with-parent", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp", "--bind", root, root}
		for _, path := range s.Writable {
			wrapped = append(wrapped, "--bind", path, path)
		}
		if !s.Network {
			wrapped = append(wrapped, "--unshare-net")
		}
		wrapped = append(wrapped, "--chdir", cwd, "--", program)
		return launch{program: s.Program, args: append(wrapped, args...), finish: done}, nil
	}
	wrapped := []string{"--die-with-parent", "--new-session", "--unshare-all", "--unshare-user", "--disable-userns"}
	if s.Network {
		wrapped = append(wrapped, "--share-net")
	}
	wrapped = append(wrapped, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp")
	for _, hidden := range hiddenPaths() {
		wrapped = append(wrapped, "--tmpfs", hidden)
	}
	if s.Network {
		// /run is hidden; the resolver's files there come back read-only.
		if dir, ok := resolverDirectory(); ok {
			wrapped = append(wrapped, "--ro-bind", dir, dir)
		}
	}
	wrapped = append(wrapped, "--bind", root, root)
	for _, path := range s.Readable {
		wrapped = append(wrapped, "--ro-bind", path, path)
	}
	for _, path := range s.Writable {
		wrapped = append(wrapped, "--bind", path, path)
	}
	l := launch{program: s.Program, finish: done}
	if s.Egress != nil && !s.Network {
		proxy, err := startEgressProxy(s.Egress.Allow)
		if err != nil {
			return launch{}, fmt.Errorf("the egress proxy cannot start: %w", err)
		}
		wrapped = append(wrapped, "--ro-bind", s.Egress.Relay, sandboxRelay, "--bind", proxy.socket(), sandboxSocket)
		program, args = sandboxRelay, append([]string{EgressRelayCommand, egressRelayAddress, sandboxSocket, "--", program}, args...)
		l.env = egressProxyVariables()
		l.finish = func(bool) string { return proxy.close() }
	}
	wrapped = append(wrapped, "--chdir", cwd, "--", program)
	l.args = append(wrapped, args...)
	return l, nil
}

// hiddenPaths are the directories an isolated sandbox covers with an empty
// filesystem: the home directories, the daemons' sockets and the mounted
// volumes. $HOME comes last, so it exists, empty and writable, however
// deep it is.
func hiddenPaths() []string {
	var hidden []string
	for _, path := range []string{"/home", "/root", "/run", "/var/run", "/var/tmp", "/mnt", "/media", "/srv"} {
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() {
			hidden = append(hidden, path)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.IsAbs(home) && filepath.Clean(home) != "/" {
		hidden = append(hidden, filepath.Clean(home))
	}
	return hidden
}

// resolverDirectory is the directory /etc/resolv.conf points into when it
// is under /run, as systemd-resolved's stub is.
func resolverDirectory() (string, bool) {
	target, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil || !strings.HasPrefix(target, "/run/") {
		return "", false
	}
	return filepath.Dir(target), true
}

// ProbeSandbox runs a trivial command under the sandbox's options, so a
// kernel or security policy that refuses it (unprivileged user namespaces
// disabled, an AppArmor rule, a bwrap too old for an option, a container
// runtime or image missing) is found at launch rather than on the model's
// first command.
func ProbeSandbox(ctx context.Context, s Sandbox) error {
	if s.Container != nil {
		return probeContainer(ctx, s)
	}
	if s.Program == "" {
		return errors.New("bwrap is not installed")
	}
	if s.Egress != nil && !s.Network && !filepath.IsAbs(s.Egress.Relay) {
		return errors.New("the egress relay needs the absolute path of axlr-tui or axlr-serve")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	probe := s
	probe.Egress = nil
	l, err := probe.launch("/", "/", "/bin/true", nil)
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, l.program, l.args...).CombinedOutput()
	l.finish(false)
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
