// Package enginesetup prepares and connects the official KMP and MADE
// packages that /update or the first-run setup downloaded: it initialises
// each engine's store with the package's own tooling, then writes the
// mcp.json entry AXLR's console drives (ids kmp and made).
package enginesetup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Registrar writes the mcp.json entry of an installed engine.
type Registrar interface {
	RegisterEngine(context.Context, application.EngineRegistration) error
}

// Installer is the EngineInstallPort of the console and the setup.
type Installer struct {
	Configuration Registrar
	Getenv        func(string) string
	// KMPApproval is kmp's approval policy: auto lets memory writes land
	// without a card; empty means manual. MADEApproval is made's: auto
	// lets the model's reads run without a card (decision B11).
	KMPApproval, MADEApproval string
	// PrepareMADE issues AXLR's work grant and publishes the driven
	// ceremonies once MADE is registered (madesetup.Preparer.Prepare).
	PrepareMADE func(context.Context) (application.OrchestratorPreparation, error)
	// run executes one setup command; tests replace it.
	run func(ctx context.Context, command string, args, env []string) ([]byte, error)
}

var _ application.EngineInstallPort = (*Installer)(nil)

// InstallEngine prepares the store and connects the engine. The child
// environment is HOME and PATH only, exactly what the registration passes
// through env_from: MADE's launcher finds its private configuration by the
// same HOME. KMP's store is not chosen here: the console resolves it per
// workspace at launch and fills a store it creates (adapters/kmpstore).
func (i *Installer) InstallEngine(ctx context.Context, engine, packageRoot string) (string, error) {
	if i.Configuration == nil || i.Getenv == nil || !filepath.IsAbs(packageRoot) {
		return "", errors.New("engine installer is not configured")
	}
	home := i.Getenv("HOME")
	if !filepath.IsAbs(home) {
		return "", errors.New("an absolute HOME is required to set up the engines")
	}
	env := []string{"HOME=" + home, "PATH=" + i.Getenv("PATH")}
	hostEnv := map[string]string{"HOME": "HOME", "PATH": "PATH"}
	launcher := filepath.Join(packageRoot, "scripts", "run-embedded-mcp.sh")
	switch engine {
	case "kmp":
		// Until 13 October 2026 the installer pinned $XDG_DATA_HOME/kmp/default
		// here for every workspace and synced the guide into it, which kept a
		// project's memory apart from what the person's other hosts wrote in
		// the same checkout. The console now chooses the store per workspace
		// at launch (the checkout's .kernel, or that per-user default outside
		// one) and writes the guide into a store it creates; the entry pins
		// only the backend.
		approval := i.KMPApproval
		if approval == "" {
			approval = "manual"
		}
		if err := i.Configuration.RegisterEngine(ctx, application.EngineRegistration{
			Engine: "kmp", Launcher: launcher, Name: "KMP", Description: "Underpass graph-temporal agent memory",
			Purpose: "memory", Approval: approval, Env: map[string]string{"KMP_MCP_BACKEND": "embedded"}, EnvFrom: hostEnv,
		}); err != nil {
			return "", err
		}
		return "connected; the memory store is chosen per workspace at launch", nil
	case "made":
		// The package's setup creates the store, its private configuration
		// and the authorization bootstrap; it reuses all three when they exist.
		if _, err := i.execute(ctx, filepath.Join(packageRoot, "scripts", "made-configure-embedded.sh"), nil, env); err != nil {
			return "", fmt.Errorf("configure the MADE store: %w", err)
		}
		madeApproval := i.MADEApproval
		if madeApproval == "" {
			madeApproval = "manual"
		}
		if err := i.Configuration.RegisterEngine(ctx, application.EngineRegistration{
			Engine: "made", Launcher: launcher, Name: "MADE", Description: "Underpass agentic ceremonies, structured reviews and human approval procedures",
			Purpose: "ceremony", Approval: madeApproval, EnvFrom: hostEnv,
		}); err != nil {
			return "", err
		}
		if i.PrepareMADE == nil {
			return "connected; prepare the ceremonies with P on MADE in /mcp", nil
		}
		prepared, err := i.PrepareMADE(ctx)
		if err != nil {
			// MADE is connected; only the grant or a definition is missing, and
			// P in /mcp repeats exactly this step.
			return "connected, but preparing the ceremonies failed: " + err.Error() + "; retry with P on MADE in /mcp", nil
		}
		if prepared.Status != "granted" && prepared.Status != "ready" {
			return "connected; ceremony preparation " + prepared.Status + ": " + prepared.Detail, nil
		}
		detail := "work identity " + prepared.WorkIdentity
		if len(prepared.Published) > 0 {
			detail += fmt.Sprintf("; %d ceremonies published", len(prepared.Published))
		}
		return detail, nil
	}
	return "", errors.New("unsupported engine")
}

func (i *Installer) execute(ctx context.Context, command string, args, env []string) ([]byte, error) {
	if i.run != nil {
		return i.run(ctx, command, args, env)
	}
	if info, err := os.Stat(command); err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return nil, fmt.Errorf("%s is missing or not executable", filepath.Base(command))
	}
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(operation, command, args...)
	cmd.Env = env
	cmd.Dir = filepath.Dir(command)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("%s failed: %s", filepath.Base(command), tail(output.String()))
	}
	return output.Bytes(), nil
}

// tail keeps the last lines of a failed command for the person to read.
func tail(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	text := strings.Join(lines, " · ")
	if len(text) > 800 {
		text = text[len(text)-800:]
	}
	if text == "" {
		return "no output"
	}
	return text
}
