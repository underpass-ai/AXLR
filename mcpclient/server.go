package mcpclient

import (
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server describes one external MCP server. Env, when set, is the complete
// environment for a stdio child; nil inherits the host process environment.
type Server struct {
	Name       ServerName
	Command    string
	Args       []string
	Env        []string
	URL        string
	HTTPClient *http.Client
}

func (s Server) Validate() error {
	if _, err := NewServerName(s.Name.String()); err != nil {
		return err
	}
	stdio, remote := s.Command != "", s.URL != ""
	if stdio == remote {
		return errors.New("configure exactly one MCP transport")
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

func (s Server) transport() (mcp.Transport, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Command != "" {
		cmd := exec.Command(s.Command, s.Args...)
		if s.Env != nil {
			cmd.Env = append([]string{}, s.Env...)
		}
		return &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil
	}
	return &mcp.StreamableClientTransport{Endpoint: s.URL, HTTPClient: s.HTTPClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil
}
