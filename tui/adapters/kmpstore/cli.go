package kmpstore

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
	"unicode/utf8"
)

// CLI runs kmp-mcp's command-line verbs, which read or write a store
// without opening it as a server (safe beside a running engine).
type CLI struct {
	// Binary is the kmp-mcp executable; empty runs nothing.
	Binary string
	// Run executes one command; nil runs it with a two-minute limit.
	Run func(ctx context.Context, command string, args, env []string) ([]byte, error)
}

// errNoBinary answers a verb on a console that found no kmp-mcp.
var errNoBinary = errors.New("no KMP binary")

func (c CLI) run(ctx context.Context, args, env []string) ([]byte, error) {
	command := c.Binary
	if command == "" {
		return nil, errNoBinary
	}
	if c.Run != nil {
		return c.Run(ctx, command, args, env)
	}
	if info, err := os.Stat(command); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("%s is missing or not executable", filepath.Base(command))
	}
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(operation, command, args...)
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("%s %s failed: %s", filepath.Base(command), strings.Join(args, " "), tail(output.String()))
	}
	return output.Bytes(), nil
}

func tail(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	text := strings.Join(lines, " · ")
	if len(text) > 800 {
		start := len(text) - 800
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		text = text[start:]
	}
	if text == "" {
		return "no output"
	}
	return text
}
