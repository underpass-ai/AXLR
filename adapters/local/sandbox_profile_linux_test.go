//go:build linux

package local

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

// The test binary stands in for axlr-tui: it is the egress relay when
// called as one, and a small client for the tests' commands.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case EgressRelayCommand:
			os.Exit(RunEgressRelay(os.Args[2:]))
		case "__sandbox-test-dial":
			os.Exit(sandboxTestDial(os.Args[2]))
		case "__sandbox-test-get":
			os.Exit(sandboxTestGet(os.Args[2]))
		}
	}
	os.Exit(m.Run())
}

func sandboxTestDial(address string) int {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		return 1
	}
	_ = conn.Close()
	fmt.Println("connected")
	return 0
}

// sandboxTestGet asks through the proxy the environment names, if any: Go
// would not send a loopback address to it on its own.
func sandboxTestGet(raw string) int {
	transport := &http.Transport{}
	if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return 2
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	response, err := client.Get(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get:", err)
		return 1
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	fmt.Printf("%d %s", response.StatusCode, body)
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// isolatedSandbox is the process profile on this machine, or a skip where
// bwrap cannot run; AXLR_SANDBOX_TESTS=required, as CI sets it, fails
// instead.
func isolatedSandbox(t *testing.T) Sandbox {
	t.Helper()
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		sandboxUnavailable(t, "bwrap is not installed")
	}
	sandbox := Sandbox{Program: bwrap, Isolate: true}
	if err := ProbeSandbox(context.Background(), sandbox); err != nil {
		sandboxUnavailable(t, err.Error())
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary lives under /tmp, which the sandbox replaces.
	sandbox.Readable = []string{filepath.Dir(executable)}
	return sandbox
}

func sandboxUnavailable(t *testing.T, why string) {
	t.Helper()
	if os.Getenv("AXLR_SANDBOX_TESTS") == "required" {
		t.Fatal(why)
	}
	t.Skip(why)
}

func runSandboxed(t *testing.T, sandbox Sandbox, root, program string, args ...string) domain.ExecResult {
	t.Helper()
	adapter := ProcessAdapter{Root: root, Env: []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + os.Getenv("HOME")}, Sandbox: &sandbox}
	result, err := adapter.Run(context.Background(), domain.ExecCommand{Program: domain.Program(program), Args: domain.Argv(args), Timeout: domain.Timeout(30 * time.Second), OutputLimit: 1 << 16})
	if err != nil {
		t.Fatalf("%s %v: %v", program, args, err)
	}
	return result
}

func TestTheProcessProfileWritesOnlyTheWorkspace(t *testing.T) {
	sandbox := isolatedSandbox(t)
	root, outside := t.TempDir(), t.TempDir()
	home := os.Getenv("HOME")
	script := `echo in > inside.txt
echo out > ` + filepath.Join(outside, "escaped.txt") + `
echo home > "$HOME/axlr-sandbox-marker"
if echo etc > /etc/axlr-sandbox-test 2>/dev/null; then echo "wrote /etc"; exit 9; fi
exit 0`
	result := runSandboxed(t, sandbox, root, "/bin/sh", "-c", script)
	if result.ExitCode != 0 {
		t.Fatalf("exit %d: %s %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if data, err := os.ReadFile(filepath.Join(root, "inside.txt")); err != nil || string(data) != "in\n" {
		t.Fatalf("the workspace write did not land: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("a write outside the workspace reached the host: %v", err)
	}
	if home != "" {
		if _, err := os.Stat(filepath.Join(home, "axlr-sandbox-marker")); !os.IsNotExist(err) {
			_ = os.Remove(filepath.Join(home, "axlr-sandbox-marker"))
			t.Fatalf("a write to $HOME reached the host: %v", err)
		}
	}
}

func TestTheProcessProfileHidesHomesAndDaemonSockets(t *testing.T) {
	sandbox := isolatedSandbox(t)
	root := t.TempDir()
	var present []string
	for _, path := range []string{"/var/run/docker.sock", "/run/docker.sock", "/run/user/" + strconv.Itoa(os.Getuid()), "/run/dbus/system_bus_socket", os.Getenv("HOME") + "/.ssh"} {
		if _, err := os.Stat(path); err == nil {
			present = append(present, path)
		}
	}
	for _, path := range present {
		result := runSandboxed(t, sandbox, root, "/bin/sh", "-c", `test ! -e "$1"`, "sh", path)
		if result.ExitCode != 0 {
			t.Errorf("%s is visible inside the sandbox", path)
		}
	}
	result := runSandboxed(t, sandbox, root, "/bin/sh", "-c", `test -d "$HOME" && test -w "$HOME" && test -z "$(ls -A "$HOME")"`)
	if result.ExitCode != 0 {
		t.Fatalf("$HOME is not an empty writable directory: %s", result.Stderr)
	}
}

func TestTheProcessProfileWithoutEgressReachesNoNetwork(t *testing.T) {
	sandbox := isolatedSandbox(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	executable, _ := os.Executable()
	for _, address := range []string{listener.Addr().String(), "1.1.1.1:443", "[2606:4700:4700::1111]:443"} {
		result := runSandboxed(t, sandbox, t.TempDir(), executable, "__sandbox-test-dial", address)
		if result.ExitCode == 0 {
			t.Errorf("the sandbox reached %s: %s", address, result.Stdout)
		}
	}
}

func TestTheProcessProfileWithAnAllowListReachesOnlyItsHosts(t *testing.T) {
	sandbox := isolatedSandbox(t)
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "allowed") }))
	defer allowed.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "other") }))
	defer other.Close()
	allowedAddress := strings.TrimPrefix(allowed.URL, "http://")
	otherAddress := strings.TrimPrefix(other.URL, "http://")
	rule, err := domain.ParseEgressRule(allowedAddress)
	if err != nil {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	sandbox.Egress = &Egress{Allow: []domain.EgressRule{rule}, Relay: executable}
	root := t.TempDir()
	result := runSandboxed(t, sandbox, root, executable, "__sandbox-test-get", allowed.URL)
	if result.ExitCode != 0 || result.Stdout != "200 allowed" {
		t.Fatalf("the allowed host: exit %d %q %q", result.ExitCode, result.Stdout, result.Stderr)
	}
	result = runSandboxed(t, sandbox, root, executable, "__sandbox-test-get", other.URL)
	if result.ExitCode == 0 || !strings.HasPrefix(result.Stdout, "403 ") {
		t.Fatalf("the host off the list: exit %d %q %q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "axlr sandbox: egress refused to "+otherAddress) {
		t.Fatalf("the refusal is not noted on stderr: %q", result.Stderr)
	}
	// The proxy is the only route: the allowed host itself is out of reach
	// without it.
	result = runSandboxed(t, sandbox, root, executable, "__sandbox-test-dial", allowedAddress)
	if result.ExitCode == 0 {
		t.Fatalf("the sandbox reached %s without the proxy", allowedAddress)
	}
}

func TestTheProcessProfileStopsAtTheTimeout(t *testing.T) {
	sandbox := isolatedSandbox(t)
	adapter := ProcessAdapter{Root: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Sandbox: &sandbox}
	start := time.Now()
	_, err := adapter.Run(context.Background(), domain.ExecCommand{Program: "/bin/sh", Args: domain.Argv{"-c", "sleep 30 & sleep 30"}, Timeout: domain.Timeout(time.Second), OutputLimit: 1024})
	fault, ok := err.(*domain.Fault)
	if !ok || fault.Status != "timed_out" {
		t.Fatalf("want timed_out, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("the sandboxed command outlived its timeout by %s", elapsed)
	}
}
