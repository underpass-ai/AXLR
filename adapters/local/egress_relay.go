package local

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// EgressRelayCommand is the first argument that runs a binary as the egress
// relay: axlr-tui and axlr-serve check for it before anything else, and a
// sandbox whose egress is an allow list starts the command through it.
const EgressRelayCommand = "__axlr-egress-relay"

// egressRelayAddress is where the relay listens inside the sandbox's own
// network namespace; the command's proxy variables name it.
const egressRelayAddress = "127.0.0.1:3128"

// egressProxyVariables point a command at the relay: curl, pip, npm, git,
// Go and most HTTP clients read them. NO_PROXY is emptied so nothing
// bypasses it; there is no other route anyway.
func egressProxyVariables() []string {
	proxy := "http://" + egressRelayAddress
	return []string{"HTTP_PROXY=" + proxy, "HTTPS_PROXY=" + proxy, "ALL_PROXY=" + proxy, "http_proxy=" + proxy, "https_proxy=" + proxy, "all_proxy=" + proxy, "NO_PROXY=", "no_proxy="}
}

// RunEgressRelay runs inside the sandbox: args are the address to listen
// on, the proxy's socket, "--", then the program and its arguments. It
// relays every connection to the address to the socket, runs the program
// with this process's standard streams and environment, and returns its
// exit status; a signal the relay receives is passed on to it.
func RunEgressRelay(args []string) int {
	if len(args) < 4 || args[2] != "--" {
		fmt.Fprintln(os.Stderr, "axlr egress relay: usage: <address> <socket> -- <program> [args...]")
		return 2
	}
	address, socket, program := args[0], args[1], args[3:]
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintln(os.Stderr, "axlr egress relay:", err)
		return 126
	}
	defer listener.Close()
	go relayConnections(listener, socket)
	cmd := exec.Command(program[0], program[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "axlr egress relay:", err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return 127
		}
		return 126
	}
	go func() {
		for received := range signals {
			_ = cmd.Process.Signal(received)
		}
	}()
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exit.ExitCode()
	}
	if err != nil {
		return 126
	}
	return 0
}

func relayConnections(listener net.Listener, socket string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go relay(conn, socket)
	}
}

func relay(conn net.Conn, socket string) {
	defer conn.Close()
	upstream, err := net.Dial("unix", socket)
	if err != nil {
		return
	}
	defer upstream.Close()
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, conn)
		if closer, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		close(copied)
	}()
	_, _ = io.Copy(conn, upstream)
	_ = conn.Close()
	<-copied
}
