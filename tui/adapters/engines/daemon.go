//go:build unix

package engines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// IdleExit is how long a daemon runs without a client: consoles that quit
// do not leave engines behind, and the next console starts one again.
const IdleExit = 30 * time.Second

// startWait bounds how long a console waits for a daemon it started.
const startWait = 5 * time.Second

// Supervisor finds or starts the daemon of an engine spec. Dir holds the
// sockets and must be private to the user; Executable is the binary whose
// --engines-serve runs a daemon (the console itself).
type Supervisor struct {
	Dir        string
	Executable string
	Version    string
}

// Socket returns the socket of a running daemon for spec, starting one
// when none answers.
func (s Supervisor) Socket(ctx context.Context, spec Spec) (string, error) {
	if err := spec.validate(); err != nil {
		return "", err
	}
	if err := privateDir(s.Dir); err != nil {
		return "", err
	}
	path := filepath.Join(s.Dir, spec.key()+".sock")
	if answers(path) {
		return path, nil
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(s.Executable, "--engines-serve", path)
	cmd.Stdin = strings.NewReader(string(data))
	// The daemon itself needs no environment; the engine gets spec.Env.
	cmd.Env = []string{}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start the engine daemon: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(startWait)
	for time.Now().Before(deadline) {
		if answers(path) {
			return path, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return "", errors.New("the engine daemon did not start")
}

// answers reports whether a daemon accepts connections on path.
func answers(path string) bool {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// privateDir creates dir 0700 or checks that an existing one is a real
// directory of this user that nobody else can enter.
func privateDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("engine socket directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("engine socket directory %s must be a directory private to this user", dir)
	}
	return nil
}

// Serve runs one daemon: it reads its spec from specIn, takes the socket's
// lock (a second daemon for the same spec exits quietly), launches the
// engine and multiplexes clients onto it until the engine exits or no
// client has been connected for idle.
func Serve(ctx context.Context, socket string, specIn io.Reader, version string, idle time.Duration) error {
	data, err := io.ReadAll(io.LimitReader(specIn, maxSpecBytes+1))
	if err != nil {
		return err
	}
	var spec Spec
	if len(data) > maxSpecBytes || json.Unmarshal(data, &spec) != nil {
		return errors.New("invalid engine spec")
	}
	if err := spec.validate(); err != nil {
		return err
	}
	if err := privateDir(filepath.Dir(socket)); err != nil {
		return err
	}
	lock, err := os.OpenFile(socket+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil // another daemon serves this spec
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer os.Remove(socket)
	defer listener.Close()
	if err := os.Chmod(socket, 0o600); err != nil {
		return err
	}
	engine := exec.Command(spec.Command, spec.Args...)
	engine.Env = append([]string{}, spec.Env...)
	stdin, err := engine.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := engine.StdoutPipe()
	if err != nil {
		return err
	}
	if err := engine.Start(); err != nil {
		return fmt.Errorf("start the engine: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		stopped := make(chan struct{})
		go func() { _ = engine.Wait(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			_ = engine.Process.Kill()
			<-stopped
		}
	}()
	m, engineDone, err := startMux(stdin, stdout, version)
	if err != nil {
		return err
	}
	conns := make(chan net.Conn)
	go func() {
		defer close(conns)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conns <- conn
		}
	}()
	var clients sync.WaitGroup
	defer clients.Wait()
	ended := make(chan struct{}, 64)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			m.close()
			return nil
		case <-engineDone:
			return nil
		case conn, ok := <-conns:
			if !ok {
				m.close()
				return nil
			}
			timer.Stop()
			clients.Add(1)
			go func() {
				defer clients.Done()
				m.serve(conn)
				ended <- struct{}{}
			}()
		case <-ended:
			if m.clientCount() == 0 {
				timer.Reset(idle)
			}
		case <-timer.C:
			if m.clientCount() == 0 {
				m.close()
				return nil
			}
		}
	}
}
