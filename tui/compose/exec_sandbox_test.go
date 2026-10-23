package compose

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

func noBwrap(string) (string, error) { return "", errors.New("not found") }

func TestExecSandboxModes(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if sandbox, why, err := ExecSandbox(ctx, "axlr-tui", storage.ExecSandboxSettings{Mode: "off"}, noBwrap, &out); sandbox != nil || why != "" || err != nil || out.Len() != 0 {
		t.Fatalf("off: %+v %v %q", sandbox, err, &out)
	}
	// auto without bwrap runs unsandboxed and says so once.
	if sandbox, why, err := ExecSandbox(ctx, "axlr-tui", storage.ExecSandboxSettings{Mode: "auto"}, noBwrap, &out); sandbox != nil || why != "bwrap is not installed" || err != nil || !strings.Contains(out.String(), "exec sandbox unavailable (bwrap is not installed): local commands run unsandboxed") {
		t.Fatalf("auto: %+v %v %q", sandbox, err, &out)
	}
	// required without bwrap refuses every command.
	out.Reset()
	sandbox, why, err := ExecSandbox(ctx, "axlr-tui", storage.ExecSandboxSettings{Mode: "required"}, noBwrap, &out)
	if err != nil || why != "bwrap is not installed" || sandbox == nil || sandbox.Unavailable != "bwrap is not installed" || !strings.Contains(out.String(), "local commands are refused") {
		t.Fatalf("required: %+v %v %q", sandbox, err, &out)
	}
	// A writable path that does not exist stops the launch.
	if _, _, err := ExecSandbox(ctx, "axlr-tui", storage.ExecSandboxSettings{Mode: "auto", Writable: []string{"/no/such/cache"}}, noBwrap, &out); err == nil || !strings.Contains(err.Error(), "/no/such/cache") {
		t.Fatalf("missing writable: %v", err)
	}
}

func TestExecSandboxUsesBwrapWhenItCanConfine(t *testing.T) {
	program, err := exec.LookPath("bwrap")
	if err != nil || local.ProbeSandbox(context.Background(), local.Sandbox{Program: program, Network: true}) != nil {
		t.Skip("bwrap cannot sandbox here")
	}
	writable := t.TempDir()
	var out bytes.Buffer
	sandbox, why, err := ExecSandbox(context.Background(), "axlr-tui", storage.ExecSandboxSettings{Mode: "required", Writable: []string{writable}}, exec.LookPath, &out)
	if err != nil || why != "" || sandbox == nil || sandbox.Program != program || !sandbox.Network || sandbox.Unavailable != "" {
		t.Fatalf("%+v %v", sandbox, err)
	}
	if !strings.Contains(out.String(), "local commands write only the workspace, "+writable+" and a private /tmp; network on") {
		t.Fatalf("output %q", &out)
	}
}
