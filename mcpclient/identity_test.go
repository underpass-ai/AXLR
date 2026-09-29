package mcpclient

import (
	"strings"
	"testing"
)

func TestMCPIdentities(t *testing.T) {
	server, err := NewServerName("files_1")
	if err != nil || server.String() != "files_1" {
		t.Fatalf("server identity %q %v", server, err)
	}
	if _, err := NewServerName("a/b"); err == nil {
		t.Fatal("invalid server identity accepted")
	}
	tool, err := NewToolName("read_file")
	if err != nil || tool.String() != "read_file" {
		t.Fatalf("tool identity %q %v", tool, err)
	}
	if _, err := NewToolName(""); err == nil {
		t.Fatal("empty tool identity accepted")
	}
	ref := ToolRef{Server: server, Name: tool}
	if ref.Server.String() != "files_1" || ref.Name.String() != "read_file" {
		t.Fatalf("bad ref %+v", ref)
	}
}

func TestToolNameMatchesMCPGrammar(t *testing.T) {
	for _, raw := range []string{"a/b", "a b", strings.Repeat("x", 129)} {
		if _, err := NewToolName(raw); err == nil {
			t.Errorf("accepted invalid MCP tool name %q", raw)
		}
	}
	for _, raw := range []string{"a.b", "a-b", "a_b"} {
		if _, err := NewToolName(raw); err != nil {
			t.Errorf("rejected valid MCP tool name %q: %v", raw, err)
		}
	}
}
