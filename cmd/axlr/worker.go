// axlr is a one-request JSON worker for the trusted-local profile.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/underpass-ai/AXLR"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("axlr", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "existing workspace root")
	profile := flags.String("profile", "", "execution profile (trusted-local)")
	var env envFlags
	flags.Var(&env, "env", "child environment KEY=VALUE; repeatable")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if *root == "" || *profile != "trusted-local" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "axlr: --root and --profile=trusted-local are required")
		return 1
	}
	executor, err := axlr.New(axlr.Config{Root: *root, Env: env})
	if err != nil {
		fmt.Fprintln(stderr, "axlr:", err)
		return 1
	}
	defer executor.Close()
	req, err := axlr.Decode(stdin)
	code := 0
	var response axlr.Response
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err != nil {
		response = axlr.ProtocolRejection(err)
		code = 2
	} else {
		response = executor.Execute(ctx, req)
	}
	b, err := axlr.MarshalResponse(response)
	if err != nil {
		fmt.Fprintln(stderr, "axlr:", err)
		return 1
	}
	b = append(b, '\n')
	n, err := stdout.Write(b)
	if err != nil || n != len(b) {
		fmt.Fprintln(stderr, "axlr: unable to write complete response")
		return 1
	}
	return code
}
