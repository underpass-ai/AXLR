package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/underpass-ai/AXLR/domain"
)

func pluginRegistration(t *testing.T, id string, allowed []domain.PluginToolName, extraEnv ...string) Registration {
	t.Helper()
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	env := append([]string{"AXLR_PLUGIN_HELPER=1"}, extraEnv...)
	r, err := NewRegistration(Manifest{ID: domain.PluginID(id), Command: command, Args: []string{"-test.run=^TestPluginHelper$"}, AllowTools: allowed}, env)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pluginCall(t *testing.T, id, name string, args string) domain.PluginCall {
	t.Helper()
	value, err := domain.NewJSONObject([]byte(args))
	if err != nil {
		t.Fatal(err)
	}
	return domain.PluginCall{Ref: domain.PluginRef{PluginID: domain.PluginID(id), ToolName: domain.PluginToolName(name)}, Arguments: value}
}

func TestManagerRejectsDuplicatePluginBeforeLaunch(t *testing.T) {
	r := pluginRegistration(t, "search", []domain.PluginToolName{"echo"})
	if _, err := NewManager([]Registration{r, r}); err == nil {
		t.Fatal("duplicate plugin accepted")
	}
}

func TestManagerDiscoversOnlyAllowedTools(t *testing.T) {
	m, err := NewManager([]Registration{pluginRegistration(t, "search", []domain.PluginToolName{"echo", "fail"})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tools, err := m.List(context.Background())
	if err != nil || len(tools) != 2 {
		t.Fatalf("tools: %+v, %v", tools, err)
	}
	for _, tool := range tools {
		if tool.Ref.PluginID != "search" || tool.Ref.ToolName == "hidden" || len(tool.InputSchema.Bytes()) == 0 {
			t.Fatalf("invalid listed tool: %+v", tool)
		}
	}
	result, err := m.Call(context.Background(), pluginCall(t, "search", "echo", `{"text":"hi"}`))
	if err != nil || result.IsError || len(result.Content) != 1 || !strings.Contains(string(result.Content[0].Bytes()), "hi") {
		t.Fatalf("call: %+v, %v", result, err)
	}
	failure, err := m.Call(context.Background(), pluginCall(t, "search", "fail", `{}`))
	if err != nil || !failure.IsError {
		t.Fatalf("tool-level failure: %+v, %v", failure, err)
	}
	for _, ref := range []struct{ id, name string }{{"missing", "echo"}, {"search", "hidden"}, {"search", "absent"}} {
		if _, err := m.Call(context.Background(), pluginCall(t, ref.id, ref.name, `{}`)); err == nil {
			t.Errorf("accepted unavailable tool %s/%s", ref.id, ref.name)
		} else {
			var fault *domain.Fault
			if !errors.As(err, &fault) || fault.Status != "rejected" {
				t.Errorf("wrong status for %s/%s: %v", ref.id, ref.name, err)
			}
		}
	}
}

func TestManagerUsesOneIsolatedChildSession(t *testing.T) {
	t.Setenv("AXLR_INHERITED_SECRET", "must-not-leak")
	logPath := filepath.Join(t.TempDir(), "starts")
	m, err := NewManager([]Registration{pluginRegistration(t, "probe", []domain.PluginToolName{"env"}, "AXLR_START_LOG="+logPath, "AXLR_EXPLICIT=present")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("plugin launched during registration")
	}
	for range 2 {
		result, err := m.Call(context.Background(), pluginCall(t, "probe", "env", `{}`))
		if err != nil || len(result.Content) != 1 {
			t.Fatalf("env call: %+v, %v", result, err)
		}
		if !strings.Contains(string(result.Content[0].Bytes()), "present:false") {
			t.Fatalf("environment leaked: %s", result.Content[0].Bytes())
		}
	}
	starts, err := os.ReadFile(logPath)
	if err != nil || string(starts) != "1\n" {
		t.Fatalf("child starts: %q, %v", starts, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call(context.Background(), pluginCall(t, "probe", "env", `{}`)); err == nil {
		t.Fatal("closed manager accepted call")
	}
}

func TestManagerCancelledCallIsNotRetried(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls")
	m, err := NewManager([]Registration{pluginRegistration(t, "slow", []domain.PluginToolName{"wait"}, "AXLR_CALL_LOG="+logPath)})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err = m.Call(ctx, pluginCall(t, "slow", "wait", `{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error: %v", err)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil || string(calls) != "1\n" {
		t.Fatalf("call count: %q, %v", calls, err)
	}
}

func TestPluginHelper(t *testing.T) {
	if os.Getenv("AXLR_PLUGIN_HELPER") != "1" {
		return
	}
	if path := os.Getenv("AXLR_START_LOG"); path != "" {
		_ = os.WriteFile(path, []byte("1\n"), 0600)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "axlr-plugin-test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Text}}, StructuredContent: map[string]any{"text": args.Text}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "fail"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "failed"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "hidden"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "env"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		text := os.Getenv("AXLR_EXPLICIT") + ":" + strconv.FormatBool(os.Getenv("AXLR_INHERITED_SECRET") != "")
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "wait"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		if path := os.Getenv("AXLR_CALL_LOG"); path != "" {
			_ = os.WriteFile(path, []byte("1\n"), 0600)
		}
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
