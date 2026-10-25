//go:build linux

package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Paths inside a container for an allow list.
const (
	containerRelay  = "/.axlr/relay"
	containerSocket = "/.axlr/egress.sock"
)

// launch runs program in a container that is removed when it ends. The
// program replaces the image's entrypoint; a bare name is found on the
// image's PATH. A container the runtime still runs after its client was
// killed is removed by finish.
func (c *Container) launch(s *Sandbox, root, cwd, program string, args []string) (launch, error) {
	name, err := containerName()
	if err != nil {
		return launch{}, err
	}
	run := []string{"run", "--rm", "-i", "--pull", "never", "--name", name, "--log-driver", "none", "--user", c.User, "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp", "--env", "HOME=/tmp"}
	if c.Kind == ContainerPodman {
		run = append(run, "--userns", "keep-id")
	}
	if !s.Network {
		run = append(run, "--network", "none")
	}
	mounts := []string{bindMount(root, root, false)}
	for _, path := range s.Readable {
		mounts = append(mounts, bindMount(path, path, true))
	}
	for _, path := range s.Writable {
		mounts = append(mounts, bindMount(path, path, false))
	}
	for _, mount := range mounts {
		run = append(run, "--mount", mount)
	}
	if c.MemoryMB > 0 {
		run = append(run, "--memory", strconv.Itoa(c.MemoryMB)+"m")
	}
	if c.CPUs > 0 {
		run = append(run, "--cpus", strconv.FormatFloat(c.CPUs, 'f', -1, 64))
	}
	if c.PIDs > 0 {
		run = append(run, "--pids-limit", strconv.Itoa(c.PIDs))
	}
	var proxy *egressProxy
	entry := program
	if s.Egress != nil && !s.Network {
		if proxy, err = startEgressProxy(s.Egress.Allow); err != nil {
			return launch{}, fmt.Errorf("the egress proxy cannot start: %w", err)
		}
		run = append(run, "--mount", bindMount(s.Egress.Relay, containerRelay, true), "--mount", bindMount(proxy.socket(), containerSocket, false))
		for _, variable := range egressProxyVariables() {
			run = append(run, "--env", variable)
		}
		entry = containerRelay
		args = append([]string{EgressRelayCommand, egressRelayAddress, containerSocket, "--", program}, args...)
	}
	run = append(run, "--workdir", cwd, "--entrypoint", entry, c.Image)
	runtime := c.Runtime
	finish := func(stopped bool) string {
		if stopped {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = exec.CommandContext(ctx, runtime, "rm", "--force", name).Run()
			cancel()
		}
		if proxy != nil {
			return proxy.close()
		}
		return ""
	}
	return launch{program: runtime, args: append(run, args...), finish: finish}, nil
}

// bindMount is a --mount value; paths are checked free of commas, which
// would split it.
func bindMount(source, target string, readOnly bool) string {
	mount := "type=bind,source=" + source + ",target=" + target
	if readOnly {
		mount += ",readonly"
	}
	return mount
}

func containerName() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "axlr-exec-" + hex.EncodeToString(random), nil
}

// probeContainer checks that the runtime answers and the image is on the
// host, and runs nothing.
func probeContainer(ctx context.Context, s Sandbox) error {
	c := s.Container
	if c.Runtime == "" {
		return fmt.Errorf("%s is not installed", c.Kind)
	}
	if strings.ContainsRune(c.Image, ',') {
		return errors.New("the container image cannot contain a comma")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, c.Runtime, "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		return fmt.Errorf("%s does not answer: %v %s", c.Kind, err, firstLine(out))
	}
	if out, err := exec.CommandContext(ctx, c.Runtime, "image", "inspect", "--format", "{{.Id}}", c.Image).CombinedOutput(); err != nil {
		return fmt.Errorf("the image %s is not on this host (AXLR does not pull; %s pull it first): %s", c.Image, c.Kind, firstLine(out))
	}
	return nil
}

// ContainerRootless reports whether the runtime runs without root: a
// rootless Podman, or a Docker daemon in rootless mode. A rootful daemon
// confines the container as well, but a flaw in it is a flaw as root.
func ContainerRootless(ctx context.Context, c Container) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	format := "{{json .SecurityOptions}}"
	if c.Kind == ContainerPodman {
		format = "{{.Host.Security.Rootless}}"
	}
	out, err := exec.CommandContext(ctx, c.Runtime, "info", "--format", format).Output()
	if err != nil {
		return false, fmt.Errorf("%s info: %w", c.Kind, err)
	}
	text := strings.TrimSpace(string(out))
	if c.Kind == ContainerPodman {
		return text == "true", nil
	}
	return strings.Contains(text, "name=rootless"), nil
}

func firstLine(out []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if len(line) > 300 {
		line = line[:300]
	}
	return line
}
