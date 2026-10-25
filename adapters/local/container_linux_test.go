//go:build linux

package local

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

func TestTheContainerProfileRunsReadOnlyWithoutPrivilegeOrNetwork(t *testing.T) {
	rule, _ := domain.ParseEgressRule("api.github.com:443")
	sandbox := Sandbox{
		Writable:  []string{"/cache"},
		Readable:  []string{"/models"},
		Container: &Container{Runtime: "/usr/bin/docker", Kind: ContainerDocker, Image: "registry.example/tools:1", MemoryMB: 512, CPUs: 1.5, PIDs: 128, User: "1000:1000"},
	}
	l, err := sandbox.launch("/work", "/work/src", "python3", []string{"-c", "print(1)"})
	if err != nil {
		t.Fatal(err)
	}
	if l.program != "/usr/bin/docker" {
		t.Fatalf("program %s", l.program)
	}
	args := strings.Join(l.args, " ")
	for _, want := range []string{"run --rm -i --pull never --name axlr-exec-", "--log-driver none --user 1000:1000 --cap-drop ALL --security-opt no-new-privileges --read-only --tmpfs /tmp --env HOME=/tmp --network none", "--mount type=bind,source=/work,target=/work --mount type=bind,source=/models,target=/models,readonly --mount type=bind,source=/cache,target=/cache", "--memory 512m --cpus 1.5 --pids-limit 128", "--workdir /work/src --entrypoint python3 registry.example/tools:1 -c print(1)"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
	if strings.Contains(args, "--userns") {
		t.Errorf("docker takes no --userns: %s", args)
	}

	sandbox.Container.Kind = ContainerPodman
	sandbox.Network = true
	l, err = sandbox.launch("/work", "/work", "sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l.args, "keep-id") || slices.Contains(l.args, "--network") {
		t.Fatalf("podman with the network: %v", l.args)
	}

	sandbox.Network = false
	sandbox.Egress = &Egress{Allow: []domain.EgressRule{rule}, Relay: "/usr/local/bin/axlr-tui"}
	l, err = sandbox.launch("/work", "/work", "curl", []string{"https://api.github.com"})
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Join(l.args, " ")
	note := l.finish(false)
	for _, want := range []string{"--network none", "type=bind,source=/usr/local/bin/axlr-tui,target=/.axlr/relay,readonly", ",target=/.axlr/egress.sock --env HTTP_PROXY=http://127.0.0.1:3128", "--entrypoint /.axlr/relay registry.example/tools:1 " + EgressRelayCommand + " 127.0.0.1:3128 /.axlr/egress.sock -- curl https://api.github.com"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
	if note != "" {
		t.Fatalf("note %q", note)
	}
}

// containerRuntime is a runtime on this machine, or a skip.
func containerRuntime(t *testing.T) Container {
	t.Helper()
	for _, kind := range []string{ContainerPodman, ContainerDocker} {
		if path, err := exec.LookPath(kind); err == nil {
			return Container{Runtime: path, Kind: kind, User: strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), PIDs: 64, MemoryMB: 256}
		}
	}
	t.Skip("neither podman nor docker is installed")
	return Container{}
}

// containerImage is a runtime and an image with /bin/sh to test with
// (AXLR_SANDBOX_TEST_IMAGE), or a skip: AXLR never pulls one.
func containerImage(t *testing.T) Container {
	t.Helper()
	c := containerRuntime(t)
	c.Image = os.Getenv("AXLR_SANDBOX_TEST_IMAGE")
	if c.Image == "" {
		t.Skip("AXLR_SANDBOX_TEST_IMAGE names no image with /bin/sh to test the container profile with")
	}
	if err := ProbeSandbox(context.Background(), Sandbox{Container: &c}); err != nil {
		t.Skip(err)
	}
	return c
}

func TestTheContainerProfileWritesOnlyTheWorkspaceAndReachesNoNetwork(t *testing.T) {
	c := containerImage(t)
	root := t.TempDir()
	sandbox := Sandbox{Container: &c}
	result := runSandboxed(t, sandbox, root, "/bin/sh", "-c", `echo in > inside.txt && if echo x > /etc/axlr 2>/dev/null; then exit 9; fi; ls /sys/class/net`)
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "lo" {
		t.Fatalf("exit %d %q %q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if data, err := os.ReadFile(root + "/inside.txt"); err != nil || string(data) != "in\n" {
		t.Fatalf("the workspace write did not land: %q %v", data, err)
	}
}

func TestTheContainerProbeNamesAMissingImage(t *testing.T) {
	c := containerRuntime(t)
	if out, err := exec.Command(c.Runtime, "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Skipf("%s does not answer: %s", c.Kind, out)
	}
	c.Image = "axlr-sandbox-test/absent:never"
	err := ProbeSandbox(context.Background(), Sandbox{Container: &c})
	if err == nil || !strings.Contains(err.Error(), "the image axlr-sandbox-test/absent:never is not on this host (AXLR does not pull") {
		t.Fatalf("%v", err)
	}
}
