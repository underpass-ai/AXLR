package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// installed is a LookPath that finds the named programs under /usr/bin.
func installed(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, each := range names {
			if each == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

// An execution's sandbox becomes the runtime's: the profile none is the
// settings' own, process is bwrap with the console's layout hidden, an
// allow list goes through this binary's relay, and container takes podman
// before docker unless one is named.
func TestExecutionSandboxesConfineEachProfile(t *testing.T) {
	settingsSandbox := &local.Sandbox{Program: "/usr/bin/bwrap", Network: true}
	x := ExecutionSandboxes{Settings: storage.ExecSandboxSettings{Mode: "auto"}, Default: settingsSandbox, LookPath: installed("bwrap", "docker", "podman"), Relay: "/opt/axlr/axlr-tui"}
	none, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileNone, Egress: domain.SandboxEgressFull})
	if err != nil || none != settingsSandbox {
		t.Fatalf("none: %+v %v", none, err)
	}
	process, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressAllowlist, Allow: []string{"api.github.com:443"}, Writable: []string{"/cache"}, Readable: []string{"/models"}})
	if err != nil || process.Program != "/usr/bin/bwrap" || !process.Isolate || process.Network || process.Egress == nil || process.Egress.Relay != "/opt/axlr/axlr-tui" || len(process.Egress.Allow) != 1 || !process.Egress.Allow[0].Allows("api.github.com", 443) || process.Container != nil || process.Writable[0] != "/cache" || process.Readable[0] != "/models" {
		t.Fatalf("process: %+v %v", process, err)
	}
	full, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressFull})
	if err != nil || !full.Network || full.Egress != nil {
		t.Fatalf("full: %+v %v", full, err)
	}
	container := domain.ExecutionSandbox{Profile: domain.SandboxProfileContainer, Egress: domain.SandboxEgressNone, Container: &domain.SandboxContainer{Image: "tools:1", MemoryMB: 256}}
	podman, err := x.Confine(container)
	if err != nil || podman.Container == nil || podman.Container.Kind != local.ContainerPodman || podman.Container.Image != "tools:1" || podman.Container.MemoryMB != 256 || !strings.Contains(podman.Container.User, ":") {
		t.Fatalf("container: %+v %v", podman, err)
	}
	container.Container.Runtime = "docker"
	docker, err := x.Confine(container)
	if err != nil || docker.Container.Kind != local.ContainerDocker || docker.Container.Runtime != "/usr/bin/docker" {
		t.Fatalf("docker: %+v %v", docker, err)
	}
	x.LookPath = installed()
	for name, sandbox := range map[string]domain.ExecutionSandbox{
		"no bwrap":   {Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressNone},
		"no runtime": container,
		"invalid":    {Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressAllowlist},
	} {
		if confined, err := x.Confine(sandbox); err == nil {
			t.Fatalf("%s: %+v", name, confined)
		}
	}
	x.LookPath, x.Relay = installed("bwrap"), ""
	if confined, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressAllowlist, Allow: []string{"pypi.org"}}); err == nil {
		t.Fatalf("an allow list without a relay: %+v", confined)
	}
}

// Settings that require the exec sandbox are a floor: an execution cannot
// choose none, nor the network when they keep commands off it.
func TestExecutionSandboxesKeepTheSettingsFloor(t *testing.T) {
	off := false
	x := ExecutionSandboxes{Settings: storage.ExecSandboxSettings{Mode: "required", Network: &off}, Default: &local.Sandbox{Program: "/usr/bin/bwrap"}, LookPath: installed("bwrap")}
	if _, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileNone, Egress: domain.SandboxEgressFull}); err == nil || !strings.Contains(err.Error(), "cannot be none") {
		t.Fatalf("none under required: %v", err)
	}
	if _, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressFull}); err == nil || !strings.Contains(err.Error(), "cannot be full") {
		t.Fatalf("full under network false: %v", err)
	}
	if _, err := x.Confine(domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressNone}); err != nil {
		t.Fatalf("a narrower sandbox: %v", err)
	}
	// The profile none probes nothing and says what its commands run under.
	x.Settings.Mode = "off"
	x.Default = nil
	how, err := x.Probe(context.Background(), domain.ExecutionSandbox{Profile: domain.SandboxProfileNone, Egress: domain.SandboxEgressFull})
	if err != nil || how != "sandbox none: local commands run unconfined" {
		t.Fatalf("%q %v", how, err)
	}
	if _, err := x.Probe(context.Background(), domain.ExecutionSandbox{Profile: domain.SandboxProfileProcess, Egress: domain.SandboxEgressNone, Writable: []string{"/axlr-sandbox-test/absent"}}); err == nil || !strings.Contains(err.Error(), "/axlr-sandbox-test/absent") {
		t.Fatalf("a missing writable path: %v", err)
	}
}
