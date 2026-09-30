package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestConfiguredMCPHelper(t *testing.T) {
	if os.Getenv("AXLR_CONFIGURED_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if len(request.ID) == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "configured-test", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo selected text", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]string{"type": "string"}}}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]string{"type": "text", "text": "echo:" + request.Params.Arguments["text"]}}}
		default:
			result = map[string]any{}
		}
		encoded, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		fmt.Println(string(encoded))
	}
	os.Exit(0)
}

func TestRunLoadsPersistentMCPAndExecutesApprovedTool(t *testing.T) {
	env := cliEnv(t)
	env["XDG_CONFIG_HOME"] = t.TempDir()
	workspace := t.TempDir()
	configDir := filepath.Join(env["XDG_CONFIG_HOME"], "axlr")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(configDir, "fixture.json")
	manifestData, _ := json.Marshal(map[string]any{"manifest_version": 1, "id": "probe", "command": command, "args": []string{"-test.run=^TestConfiguredMCPHelper$"}, "allow_tools": []string{"*"}})
	if err := os.WriteFile(manifest, manifestData, 0600); err != nil {
		t.Fatal(err)
	}
	configData, _ := json.Marshal(map[string]any{"version": 1, "plugins": []any{map[string]any{"manifest": manifest, "env": map[string]string{"AXLR_CONFIGURED_MCP_HELPER": "1"}}}})
	if err := os.WriteFile(filepath.Join(configDir, "mcp.json"), configData, 0600); err != nil {
		t.Fatal(err)
	}
	alias := "echo"
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(request.Body)
		var chunk string
		if calls == 1 {
			if !bytes.Contains(body, []byte(alias)) || !bytes.Contains(body, []byte("probe/echo")) {
				t.Errorf("configured MCP tool not offered to model: %s", body)
			}
			chunk = `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"mcp-1","type":"function","function":{"name":"` + alias + `","arguments":"{\"text\":\"hi\"}"}}]},"finish_reason":"tool_calls"}]}`
		} else {
			if !bytes.Contains(body, []byte("echo:hi")) || !bytes.Contains(body, []byte("mcp-1")) {
				t.Errorf("MCP tool result not returned to model: %s", body)
			}
			chunk = `{"choices":[{"index":0,"delta":{"content":"MCP finished"},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	var output bytes.Buffer
	code := run(context.Background(), []string{"--root", workspace, "--model", "fixture/model"}, func(key string) string { return env[key] }, func(model tea.Model) error {
		app := model.(terminal.AppModel)
		app.Composer.Input.SetValue("use MCP")
		next, cmd := app.Update(terminal.ControlIntent("send"))
		app = drain(t, next, cmd)
		if app.Header.State.Status != domain.StatusApproval || calls != 1 {
			t.Fatalf("MCP approval not shown: %+v calls=%d", app.Status, calls)
		}
		next, cmd = app.Update(terminal.ControlIntent("approve"))
		app = drain(t, next, cmd)
		if app.Header.State.Status != domain.StatusComplete || calls != 2 || app.Status.Error != "" || !strings.Contains(app.Transcript.Viewport.GetContent(), "MCP finished") {
			t.Fatalf("MCP turn did not finish: %+v calls=%d transcript=%q", app.Status, calls, app.Transcript.Viewport.GetContent())
		}
		return nil
	}, &output)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, &output)
	}
}
