//go:build linux

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/dto"
)

// sandboxOrSkip is a sandbox for the test, or a skip where bubblewrap is
// missing or the kernel or its policy refuses it, as on CI runners that
// restrict unprivileged user namespaces.
func sandboxOrSkip(t *testing.T, sandbox local.Sandbox) *local.Sandbox {
	t.Helper()
	program, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bwrap is not installed")
	}
	sandbox.Program = program
	if err := local.ProbeSandbox(context.Background(), sandbox); err != nil {
		t.Skipf("bwrap cannot sandbox here: %v", err)
	}
	return &sandbox
}

func sandboxExec(t *testing.T, e *Executor, program string, args ...string) dto.Response {
	t.Helper()
	arguments, _ := json.Marshal(map[string]any{"program": program, "args": args, "timeout_ms": 20000})
	return e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "s", Tool: "exec", Arguments: arguments})
}

// outsideDir is a directory outside both the workspace and /tmp, which
// the sandbox replaces with a private one.
func outsideDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".sandbox-outside-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestSandboxedExecWritesOnlyTheWorkspaceAndWritablePaths(t *testing.T) {
	writable := outsideDir(t)
	readOnly := outsideDir(t)
	sandbox := sandboxOrSkip(t, local.Sandbox{Network: true, Writable: []string{writable}})
	root := t.TempDir()
	e, err := newTestExecutor(t, Config{Root: root, Env: []string{"PATH=/usr/bin:/bin"}, Sandbox: sandbox})
	if err != nil {
		t.Fatal(err)
	}
	r := sandboxExec(t, e, "/bin/sh", "-c", "printf in > inside.txt && printf w > "+filepath.Join(writable, "w.txt"))
	if out, ok := r.Output.(dto.ExecOutput); r.Status != "completed" || !ok || out.ExitCode != 0 {
		t.Fatalf("workspace and writable path: %+v", r)
	}
	for _, path := range []string{filepath.Join(root, "inside.txt"), filepath.Join(writable, "w.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was not written: %v", path, err)
		}
	}
	// Outside them the filesystem is read-only, and the program's own exit
	// status comes back through bwrap.
	r = sandboxExec(t, e, "/bin/sh", "-c", "printf x > "+filepath.Join(readOnly, "x.txt")+" 2>/dev/null || exit 7")
	if out, ok := r.Output.(dto.ExecOutput); r.Status != "completed" || !ok || out.ExitCode != 7 {
		t.Fatalf("read-only path: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(readOnly, "x.txt")); !os.IsNotExist(err) {
		t.Fatalf("the sandbox wrote outside the workspace: %v", err)
	}
	// /tmp is the command's own: what it writes there never reaches the host.
	leak := filepath.Join(os.TempDir(), fmt.Sprintf("axlr-sandbox-%d", time.Now().UnixNano()))
	r = sandboxExec(t, e, "/bin/sh", "-c", "printf x > "+leak)
	if out, ok := r.Output.(dto.ExecOutput); r.Status != "completed" || !ok || out.ExitCode != 0 {
		t.Fatalf("private /tmp: %+v", r)
	}
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		os.Remove(leak)
		t.Fatal("a write to /tmp reached the host")
	}
}

func TestSandboxedExecCanBeCutOffTheNetwork(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("needs bash for /dev/tcp")
	}
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
			conn.Close()
		}
	}()
	dial := fmt.Sprintf("exec 3<>/dev/tcp/127.0.0.1/%d", listener.Addr().(*net.TCPAddr).Port)
	for _, network := range []bool{true, false} {
		sandbox := sandboxOrSkip(t, local.Sandbox{Network: network})
		e, err := newTestExecutor(t, Config{Root: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Sandbox: sandbox})
		if err != nil {
			t.Fatal(err)
		}
		r := sandboxExec(t, e, "/bin/bash", "-c", dial)
		out, ok := r.Output.(dto.ExecOutput)
		if r.Status != "completed" || !ok || (out.ExitCode == 0) != network {
			t.Fatalf("network %v: %+v", network, r)
		}
	}
}

// Cancellation and timeouts still kill the whole tree: bwrap stays in the
// process group the adapter kills.
func TestSandboxedExecTimesOutPromptly(t *testing.T) {
	sandbox := sandboxOrSkip(t, local.Sandbox{Network: true})
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Sandbox: sandbox})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "t", Tool: "exec", Arguments: json.RawMessage(`{"program":"/bin/sh","args":["-c","sleep 30 & wait"],"timeout_ms":300}`)})
	if r.Status != "timed_out" || time.Since(started) > 10*time.Second {
		t.Fatalf("%+v after %v", r, time.Since(started))
	}
}
