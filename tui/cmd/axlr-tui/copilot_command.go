package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	copilotprovider "github.com/underpass-ai/AXLR/tui/adapters/copilot"
)

// runCopilotCommand delegates OAuth to the unmodified official CLI. The SDK
// consumes its credential store (or gh's login); AXLR never copies tokens.
func runCopilotCommand(ctx context.Context, args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "login" && args[0] != "status") {
		fmt.Fprintln(stderr, "usage: axlr-tui copilot login [--cli PATH] [--host URL] | status [--cli PATH]")
		return 2
	}
	command := args[0]
	flags := flag.NewFlagSet("copilot "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	cli := flags.String("cli", getenv("AXLR_COPILOT_CLI"), "path to the official Copilot CLI")
	host := flags.String("host", "", "GitHub Enterprise Cloud host URL (login only)")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || (command == "status" && *host != "") {
		return 2
	}
	if *cli == "" {
		*cli = "copilot"
	}
	path, err := exec.LookPath(*cli)
	if err != nil {
		fmt.Fprintln(stderr, "Copilot CLI not found; install it or set AXLR_COPILOT_CLI to its executable path")
		return 1
	}
	if command == "login" {
		argv := []string{"login", "--device-code"}
		if *host != "" {
			argv = append(argv, "--host", *host)
		}
		cmd := exec.CommandContext(ctx, path, argv...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
		// Avoid a token or a BYOK override silently choosing another account.
		cmd.Env = copilotprovider.ProcessEnv(os.Environ())
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(stderr, "Copilot device login did not complete")
			return 1
		}
		return 0
	}
	return copilotAuthStatus(ctx, path, stdout, stderr)
}

func copilotAuthStatus(ctx context.Context, path string, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := copilot.NewClient(&copilot.ClientOptions{Connection: copilot.StdioConnection{Path: path}, Env: copilotprovider.ProcessEnv(os.Environ()), LogLevel: "error"})
	if err := client.Start(ctx); err != nil {
		fmt.Fprintln(stderr, "Could not start Copilot CLI; check that the executable is compatible with the SDK")
		return 1
	}
	defer client.Stop()
	status, err := client.GetAuthStatus(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(stderr, "Copilot authentication check timed out")
		} else {
			fmt.Fprintln(stderr, "Could not check Copilot authentication")
		}
		return 1
	}
	if !status.IsAuthenticated {
		fmt.Fprintln(stdout, "Not authenticated; run axlr-tui copilot login")
		return 1
	}
	fmt.Fprint(stdout, "Authenticated")
	if status.Login != nil && *status.Login != "" {
		fmt.Fprintf(stdout, " as %s", *status.Login)
	}
	if status.AuthType != nil && *status.AuthType != "" {
		fmt.Fprintf(stdout, " (authentication: %s)", *status.AuthType)
	}
	fmt.Fprintln(stdout, ". Model access and the organization's Copilot policy are checked on use.")
	return 0
}
