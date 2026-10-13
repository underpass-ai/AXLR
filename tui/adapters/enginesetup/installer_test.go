package enginesetup

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

type registrar struct {
	got []application.EngineRegistration
	err error
}

func (r *registrar) RegisterEngine(_ context.Context, registration application.EngineRegistration) error {
	r.got = append(r.got, registration)
	return r.err
}

type call struct {
	command string
	args    []string
	env     []string
}

func installer(t *testing.T, r *registrar, calls *[]call, fail error) *Installer {
	t.Helper()
	env := map[string]string{"HOME": "/home/person", "PATH": "/usr/bin:/bin", "OPENROUTER_API_KEY": "secret", "XDG_CONFIG_HOME": "/elsewhere"}
	return &Installer{
		Configuration: r,
		DataBase:      "/home/person/.local/share",
		Getenv:        func(name string) string { return env[name] },
		KMPApproval:   "auto",
		run: func(_ context.Context, command string, args, env []string) ([]byte, error) {
			*calls = append(*calls, call{command, args, env})
			return nil, fail
		},
	}
}

func TestInstallKMPSynchronisesTheGuideIntoThePinnedStoreBeforeRegistering(t *testing.T) {
	r := &registrar{}
	var calls []call
	root := filepath.Join(t.TempDir(), "kmp-0.25.0-abc")
	detail, err := installer(t, r, &calls, nil).InstallEngine(context.Background(), "kmp", root)
	if err != nil {
		t.Fatal(err)
	}
	store := "/home/person/.local/share/kmp/default"
	if detail != "memory store "+store {
		t.Fatalf("detail %q", detail)
	}
	want := call{filepath.Join(root, "bin", "kmp-mcp"), []string{"guide", "sync", "--plugin-root", root}, []string{"HOME=/home/person", "PATH=/usr/bin:/bin", "KMP_MCP_BACKEND=embedded", "KMP_MCP_DATA_DIR=" + store}}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("calls %+v", calls)
	}
	if len(r.got) != 1 {
		t.Fatalf("registrations %+v", r.got)
	}
	got := r.got[0]
	if got.Engine != "kmp" || got.Launcher != filepath.Join(root, "scripts", "run-embedded-mcp.sh") || got.Purpose != "memory" || got.Approval != "auto" ||
		!reflect.DeepEqual(got.Env, map[string]string{"KMP_MCP_BACKEND": "embedded", "KMP_MCP_DATA_DIR": store}) ||
		!reflect.DeepEqual(got.EnvFrom, map[string]string{"HOME": "HOME", "PATH": "PATH"}) {
		t.Fatalf("registration %+v", got)
	}
}

func TestInstallMADEConfiguresWithTheLauncherEnvironmentThenPrepares(t *testing.T) {
	r := &registrar{}
	var calls []call
	root := filepath.Join(t.TempDir(), "made-0.10.0-abc")
	i := installer(t, r, &calls, nil)
	prepared := false
	i.PrepareMADE = func(context.Context) (application.MADEPreparation, error) {
		if len(r.got) != 1 {
			t.Fatal("prepared before MADE was registered")
		}
		prepared = true
		return application.MADEPreparation{Status: "granted", WorkIdentity: "axlr-work-1", Published: []string{"axlr_debug 2.0", "axlr_plan 1.0"}}, nil
	}
	detail, err := i.InstallEngine(context.Background(), "made", root)
	if err != nil || !prepared {
		t.Fatal(err)
	}
	if detail != "work identity axlr-work-1; 2 ceremonies published" {
		t.Fatalf("detail %q", detail)
	}
	// Only HOME and PATH: the launcher gets the same two through env_from,
	// so it finds the configuration this step writes under HOME.
	want := call{filepath.Join(root, "scripts", "made-configure-embedded.sh"), nil, []string{"HOME=/home/person", "PATH=/usr/bin:/bin"}}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("calls %+v", calls)
	}
	got := r.got[0]
	if got.Engine != "made" || got.Purpose != "ceremony" || got.Approval != "manual" || len(got.Env) != 0 || !reflect.DeepEqual(got.EnvFrom, map[string]string{"HOME": "HOME", "PATH": "PATH"}) {
		t.Fatalf("registration %+v", got)
	}
}

func TestInstallMADEStaysConnectedWhenPreparationFails(t *testing.T) {
	r := &registrar{}
	var calls []call
	i := installer(t, r, &calls, nil)
	i.PrepareMADE = func(context.Context) (application.MADEPreparation, error) {
		return application.MADEPreparation{}, errors.New("grant refused")
	}
	detail, err := i.InstallEngine(context.Background(), "made", t.TempDir())
	if err != nil || len(r.got) != 1 || !strings.Contains(detail, "grant refused") || !strings.Contains(detail, "/mcp") {
		t.Fatalf("%q %v", detail, err)
	}
}

func TestInstallStopsBeforeRegisteringWhenTheStoreSetupFails(t *testing.T) {
	for _, engine := range []string{"kmp", "made"} {
		r := &registrar{}
		var calls []call
		_, err := installer(t, r, &calls, errors.New("boom")).InstallEngine(context.Background(), engine, t.TempDir())
		if err == nil || len(r.got) != 0 {
			t.Fatalf("%s: %v %+v", engine, err, r.got)
		}
	}
}

func TestInstallRefusesAnUnknownEngineAndARelativeHome(t *testing.T) {
	r := &registrar{}
	var calls []call
	i := installer(t, r, &calls, nil)
	if _, err := i.InstallEngine(context.Background(), "other", t.TempDir()); err == nil {
		t.Fatal("unknown engine accepted")
	}
	i.Getenv = func(string) string { return "relative" }
	if _, err := i.InstallEngine(context.Background(), "kmp", t.TempDir()); err == nil || len(calls) != 0 {
		t.Fatal("relative HOME accepted")
	}
}

func TestExecuteReportsTheEndOfAFailedCommand(t *testing.T) {
	if got := tail("a\nb\nc\nd\ne\nf\ng\nh"); got != "c · d · e · f · g · h" {
		t.Fatalf("%q", got)
	}
	i := &Installer{}
	if _, err := i.execute(context.Background(), filepath.Join(t.TempDir(), "missing.sh"), nil, nil); err == nil || !strings.Contains(err.Error(), "missing.sh") {
		t.Fatal(err)
	}
}
