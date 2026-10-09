package mcpclient

import (
	"net/http"
	"testing"
)

func TestServerValidate(t *testing.T) {
	tests := []struct {
		name   string
		server Server
		valid  bool
	}{
		{"stdio", Server{Name: "files", Command: "/usr/bin/mcp", Args: []string{"--serve"}}, true},
		{"http", Server{Name: "web", URL: "https://example.com/mcp", HTTPClient: &http.Client{}}, true},
		{"empty name", Server{Command: "mcp"}, false},
		{"bad name", Server{Name: "a/b", Command: "mcp"}, false},
		{"no transport", Server{Name: "x"}, false},
		{"mixed transport", Server{Name: "x", Command: "mcp", URL: "https://example.com/mcp"}, false},
		{"bad URL", Server{Name: "x", URL: "ftp://example.com/mcp"}, false},
		{"URL credential", Server{Name: "x", URL: "https://u:p@example.com/mcp"}, false},
		{"invalid env", Server{Name: "x", Command: "mcp", Env: []string{"BAD"}}, false},
		{"socket", Server{Name: "x", Socket: "/run/user/1/axlr/engines/k.sock"}, true},
		{"relative socket", Server{Name: "x", Socket: "k.sock"}, false},
		{"socket and command", Server{Name: "x", Socket: "/s.sock", Command: "mcp"}, false},
		{"socket with env", Server{Name: "x", Socket: "/s.sock", Env: []string{"A=1"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}
