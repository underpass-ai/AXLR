package mcpclient

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server describes one external MCP server. Env, when set, is the complete
// environment for a stdio child; nil inherits the host process environment.
// Socket is a unix socket where a process shared with other clients serves
// MCP as a stdio child would, one JSON message per line.
type Server struct {
	Name       ServerName
	Command    string
	Args       []string
	Env        []string
	URL        string
	Socket     string
	HTTPClient *http.Client
	// Stderr, when set, receives a stdio child's standard error, which is
	// discarded otherwise. Other transports have none.
	Stderr io.Writer
}

func (s Server) Validate() error {
	if _, err := NewServerName(s.Name.String()); err != nil {
		return err
	}
	stdio, remote, socket := s.Command != "", s.URL != "", s.Socket != ""
	if btoi(stdio)+btoi(remote)+btoi(socket) != 1 {
		return errors.New("configure exactly one MCP transport")
	}
	if socket {
		if !filepath.IsAbs(s.Socket) || len(s.Args) != 0 || s.Env != nil || s.HTTPClient != nil {
			return errors.New("a socket transport needs an absolute path and no command options")
		}
		return nil
	}
	if stdio {
		if strings.ContainsRune(s.Command, 0) {
			return errors.New("invalid MCP command")
		}
		for _, arg := range s.Args {
			if strings.ContainsRune(arg, 0) {
				return errors.New("invalid MCP command argument")
			}
		}
		for _, entry := range s.Env {
			key, _, ok := strings.Cut(entry, "=")
			if !ok || key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(entry, 0) {
				return errors.New("invalid MCP child environment")
			}
		}
		if s.HTTPClient != nil {
			return errors.New("HTTP client requires HTTP transport")
		}
		return nil
	}
	if len(s.Args) != 0 || s.Env != nil {
		return errors.New("command options require stdio transport")
	}
	u, err := url.Parse(s.URL)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("invalid MCP HTTP endpoint")
	}
	return nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s Server) transport() (mcp.Transport, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Socket != "" {
		conn, err := net.DialTimeout("unix", s.Socket, 5*time.Second)
		if err != nil {
			return nil, err
		}
		return &mcp.IOTransport{Reader: conn, Writer: conn}, nil
	}
	if s.Command != "" {
		cmd := exec.Command(s.Command, s.Args...)
		if s.Env != nil {
			cmd.Env = append([]string{}, s.Env...)
		}
		if s.Stderr != nil {
			// A grandchild holding the pipe must not keep Wait from returning.
			cmd.Stderr, cmd.WaitDelay = s.Stderr, time.Second
		}
		return &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil
	}
	return &mcp.StreamableClientTransport{Endpoint: s.URL, HTTPClient: s.HTTPClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil
}
