package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/mcpclient"
)

func TestRegisterThirdPartyHTTPServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "third-party", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	manager, err := NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	registration, err := NewRegistration(Manifest{ID: "remote", URL: httpServer.URL, AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	tools, err := manager.ListServer(context.Background(), "remote")
	if err != nil || len(tools) != 1 || tools[0].Ref.ToolName != "ping" {
		t.Fatalf("HTTP tools: %+v %v", tools, err)
	}
}

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

func TestManagerWildcardDiscoversAndCallsAdvertisedTools(t *testing.T) {
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	registration, err := NewRegistration(Manifest{ID: "all", Command: command, Args: []string{"-test.run=^TestPluginHelper$"}, AllowAll: true}, []string{"AXLR_PLUGIN_HELPER=1"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager([]Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	tools, err := manager.List(context.Background())
	if err != nil || len(tools) < 3 {
		t.Fatalf("wildcard tool list: %+v %v", tools, err)
	}
	if _, err := manager.Call(context.Background(), pluginCall(t, "all", "hidden", `{}`)); err != nil {
		t.Fatalf("advertised wildcard tool rejected: %v", err)
	}
	if _, err := manager.Call(context.Background(), pluginCall(t, "all", "not_advertised", `{}`)); err == nil {
		t.Fatal("wildcard called tool not advertised by server")
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

func TestManagerRelaunchesAServerThatDied(t *testing.T) {
	m, err := NewManager([]Registration{pluginRegistration(t, "flaky", []domain.PluginToolName{"echo", "die"})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.Call(ctx, pluginCall(t, "flaky", "echo", `{"text":"one"}`)); err != nil {
		t.Fatalf("first echo: %v", err)
	}
	if _, err := m.Call(ctx, pluginCall(t, "flaky", "die", `{}`)); err == nil {
		t.Fatal("call that killed the server succeeded")
	}
	result, err := m.Call(ctx, pluginCall(t, "flaky", "echo", `{"text":"again"}`))
	if err != nil {
		t.Fatalf("server was not relaunched after it died: %v", err)
	}
	if len(result.Content) != 1 || !strings.Contains(string(result.Content[0].Bytes()), "again") {
		t.Fatalf("relaunched server answered %+v", result)
	}
	tools, err := m.ListServer(ctx, "flaky")
	if err != nil || len(tools) != 2 {
		t.Fatalf("relaunched server tools: %+v %v", tools, err)
	}
}

// A connection the manager never recorded, as left when the deadline expired
// right after Connect succeeded, is replaced instead of blocking the server.
func TestManagerReplacesAnUnrecordedSession(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "remote", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	registration, err := NewRegistration(Manifest{ID: "remote", URL: httpServer.URL, AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager([]Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.client.Connect(context.Background(), mcpclient.Server{Name: "remote", URL: httpServer.URL}); err != nil {
		t.Fatal(err)
	}
	tools, err := m.ListServer(context.Background(), "remote")
	if err != nil || len(tools) != 1 {
		t.Fatalf("server blocked by an unrecorded session: %+v %v", tools, err)
	}
}

func TestManagerPreservesLargeJSONInteger(t *testing.T) {
	m, err := NewManager([]Registration{pluginRegistration(t, "numbers", []domain.PluginToolName{"number"})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	result, err := m.Call(context.Background(), pluginCall(t, "numbers", "number", `{"n":9007199254740993}`))
	if err != nil || len(result.Content) != 1 || !strings.Contains(string(result.Content[0].Bytes()), "9007199254740993") {
		t.Fatalf("numeric argument changed: %+v, %v", result, err)
	}
}

func TestManagerPreservesLargeStructuredResultInteger(t *testing.T) {
	m, err := NewManager([]Registration{pluginRegistration(t, "numbers", []domain.PluginToolName{"large_result"})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	result, err := m.Call(context.Background(), pluginCall(t, "numbers", "large_result", `{}`))
	if err != nil || result.StructuredContent == nil || string(result.StructuredContent.Bytes()) != `{"id":9007199254740993}` {
		t.Fatalf("structured integer changed: %+v, %v", result, err)
	}
}

func TestManagerCloseInterruptsActivePluginCall(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	m, err := NewManager([]Registration{pluginRegistration(t, "slow", []domain.PluginToolName{"wait"}, "AXLR_CALL_LOG="+callLog)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callDone := make(chan error, 1)
	go func() { _, err := m.Call(ctx, pluginCall(t, "slow", "wait", `{}`)); callDone <- err }()
	waitForFile(t, callLog)
	closeDone := make(chan error, 1)
	go func() { closeDone <- m.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		<-closeDone
		t.Fatal("Close blocked behind active plugin call")
	}
	if err := <-callDone; err == nil {
		t.Fatal("closed manager left active call successful")
	}
}

func TestManagerCloseInterruptsPluginInitialization(t *testing.T) {
	startLog := filepath.Join(t.TempDir(), "init")
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistration(Manifest{ID: "hung", Command: command, Args: []string{"-test.run=^TestHangingPluginHelper$"}, AllowTools: []domain.PluginToolName{"wait"}}, []string{"AXLR_HANG_INIT=1", "AXLR_INIT_LOG=" + startLog})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}
	listDone := make(chan error, 1)
	go func() { _, err := m.List(context.Background()); listDone <- err }()
	waitForFile(t, startLog)
	closeDone := make(chan error, 1)
	go func() { closeDone <- m.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked behind plugin initialization")
	}
	if err := <-listDone; err == nil {
		t.Fatal("plugin initialization succeeded after manager closed")
	}
}

func TestQueuedPluginCallHonorsItsDeadline(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	m, err := NewManager([]Registration{pluginRegistration(t, "slow", []domain.PluginToolName{"wait", "env"}, "AXLR_CALL_LOG="+callLog)})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstDone := make(chan error, 1)
	go func() { _, err := m.Call(firstCtx, pluginCall(t, "slow", "wait", `{}`)); firstDone <- err }()
	waitForFile(t, callLog)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	secondDone := make(chan error, 1)
	go func() { _, err := m.Call(ctx, pluginCall(t, "slow", "env", `{}`)); secondDone <- err }()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("queued call error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		cancelFirst()
		<-secondDone
		t.Fatal("queued call ignored its deadline")
	}
	cancelFirst()
	<-firstDone
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin call did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPluginHelper(t *testing.T) {
	if os.Getenv("AXLR_PLUGIN_HELPER") != "1" {
		return
	}
	if path := os.Getenv("AXLR_START_LOG"); path != "" {
		_ = os.WriteFile(path, []byte("1\n"), 0600)
	}
	if note := os.Getenv("AXLR_STDERR_NOTE"); note != "" {
		_, _ = os.Stderr.WriteString(note + "\n")
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
	server.AddTool(&mcp.Tool{Name: "number", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]json.RawMessage
		if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(args["n"])}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "large_result", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{}, StructuredContent: json.RawMessage(`{"id":9007199254740993}`)}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "die"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		os.Exit(1)
		return nil, nil, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestHangingPluginHelper(t *testing.T) {
	if os.Getenv("AXLR_HANG_INIT") != "1" {
		return
	}
	_ = os.WriteFile(os.Getenv("AXLR_INIT_LOG"), []byte("started\n"), 0600)
	time.Sleep(30 * time.Second)
}

func TestManagerListServerIsolatesFailuresAndHonorsLifecycle(t *testing.T) {
	good := pluginRegistration(t, "good", []domain.PluginToolName{"echo"})
	bad, err := NewRegistration(Manifest{ID: "bad", Command: filepath.Join(t.TempDir(), "nonexistent-axlr-mcp"), AllowTools: []domain.PluginToolName{"echo"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager([]Registration{bad, good})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.ListServer(context.Background(), "bad"); err == nil {
		t.Fatal("missing server appeared available")
	}
	tools, err := m.ListServer(context.Background(), "good")
	if err != nil || len(tools) != 1 || tools[0].Ref.PluginID != "good" {
		t.Fatalf("healthy server lost: %+v %v", tools, err)
	}
	if _, err := m.ListServer(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown server accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.ListServer(ctx, "good"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ListServer(context.Background(), "good"); err == nil {
		t.Fatal("closed manager reused")
	}
}
