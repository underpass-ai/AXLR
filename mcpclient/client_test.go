package mcpclient

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer(t *testing.T, name string, pageSize int) (*mcp.Server, mcp.Transport) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1.0.0"}, &mcp.ServerOptions{PageSize: pageSize})
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo text"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name + ":" + args.Text}}, StructuredContent: map[string]any{"from": name}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "fail"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "remote failure"}}}, nil, nil
	})
	left, right := mcp.NewInMemoryTransports()
	session, err := server.Connect(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return server, right
}

func TestDiscoverAndCallAcrossServers(t *testing.T) {
	client := New()
	defer client.Close()
	_, alpha := testServer(t, "alpha", 1)
	_, beta := testServer(t, "beta", 1)
	if err := client.connect(context.Background(), "alpha", alpha); err != nil {
		t.Fatal(err)
	}
	if err := client.connect(context.Background(), "beta", beta); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		tools, err := client.ListTools(context.Background(), ServerName(name))
		if err != nil {
			t.Fatal(err)
		}
		if len(tools) != 2 {
			t.Fatalf("%s: got %d tools", name, len(tools))
		}
		for _, tool := range tools {
			if tool.Ref.Server.String() != name || tool.Ref.Name == "" || len(tool.InputSchema) == 0 {
				t.Fatalf("bad tool: %+v", tool)
			}
		}
		result, err := client.Call(context.Background(), ToolRef{Server: ServerName(name), Name: "echo"}, map[string]any{"text": "hello"})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError || len(result.Content) != 1 || !strings.Contains(string(result.Content[0]), name+":hello") {
			t.Fatalf("bad result: %+v", result)
		}
		var structured map[string]string
		if err := json.Unmarshal(result.StructuredContent, &structured); err != nil || structured["from"] != name {
			t.Fatalf("structured: %v %+v", err, structured)
		}
		failure, err := client.Call(context.Background(), ToolRef{Server: ServerName(name), Name: "fail"}, nil)
		if err != nil || !failure.IsError {
			t.Fatalf("tool error: %+v %v", failure, err)
		}
	}
	if err := client.connect(context.Background(), "alpha", alpha); err == nil {
		t.Fatal("duplicate server accepted")
	}
	if _, err := client.ListTools(context.Background(), "missing"); err == nil {
		t.Fatal("missing server accepted")
	}
	if _, err := client.Call(context.Background(), ToolRef{Server: "alpha"}, nil); err == nil {
		t.Fatal("empty tool accepted")
	}
}

func TestHTTPTransport(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "remote", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	client := New()
	defer client.Close()
	if err := client.Connect(context.Background(), Server{Name: "remote", URL: httpServer.URL}); err != nil {
		t.Fatal(err)
	}
	tools, err := client.ListTools(context.Background(), "remote")
	if err != nil || len(tools) != 1 {
		t.Fatalf("tools %v %v", tools, err)
	}
	result, err := client.Call(context.Background(), ToolRef{Server: "remote", Name: "ping"}, nil)
	if err != nil || len(result.Content) != 1 || !strings.Contains(string(result.Content[0]), "pong") {
		t.Fatalf("call %+v %v", result, err)
	}
}

func TestCallCancellation(t *testing.T) {
	client := New()
	defer client.Close()
	server := mcp.NewServer(&mcp.Implementation{Name: "slow", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "wait"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	left, right := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	if err := client.connect(context.Background(), "slow", right); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Call(ctx, ToolRef{Server: "slow", Name: "wait"}, nil); err == nil {
		t.Fatal("cancelled call succeeded")
	}
}

func TestStdioTransport(t *testing.T) {
	client := New()
	defer client.Close()
	err := client.Connect(context.Background(), Server{Name: "child", Command: os.Args[0], Args: []string{"-test.run=^TestMCPStdioHelper$"}, Env: []string{"AXLR_MCP_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Call(context.Background(), ToolRef{Server: "child", Name: "ping"}, nil)
	if err != nil || len(result.Content) != 1 || !strings.Contains(string(result.Content[0]), "pong") {
		t.Fatalf("stdio call: %+v %v", result, err)
	}
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("AXLR_MCP_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "child", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestClientLifecycleAndArguments(t *testing.T) {
	client := New()
	if err := client.Connect(context.Background(), Server{Name: "bad"}); err == nil {
		t.Fatal("invalid server accepted")
	}
	_, transport := testServer(t, "live", 0)
	if err := client.connect(context.Background(), "live", transport); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), ToolRef{Server: "live", Name: "echo"}, map[string]any{"bad": math.NaN()}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(context.Background(), "live"); err == nil {
		t.Fatal("closed client listed tools")
	}
	if err := client.Connect(context.Background(), Server{Name: "later", Command: "missing"}); err == nil {
		t.Fatal("closed client connected")
	}
}

func TestRepeatedToolCursorStops(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "loop", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "dummy"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				return &mcp.ListToolsResult{NextCursor: "same", Tools: []*mcp.Tool{}}, nil
			}
			return next(ctx, method, req)
		}
	})
	left, right := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := New()
	defer client.Close()
	if err := client.connect(context.Background(), "loop", right); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(context.Background(), "loop"); err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("expected cursor error, got %v", err)
	}
}

func TestToolListChangedInvalidatesDiscovery(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic", Version: "1.0.0"}, &mcp.ServerOptions{
		SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.TTLMs = 60_000 },
	})
	add := func(name string) {
		mcp.AddTool(server, &mcp.Tool{Name: name}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	}
	add("first")
	left, right := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := New()
	defer client.Close()
	if err := client.connect(context.Background(), "dynamic", right); err != nil {
		t.Fatal(err)
	}
	before, err := client.ListTools(context.Background(), "dynamic")
	if err != nil || len(before) != 1 {
		t.Fatalf("initial list %v %v", before, err)
	}
	add("second")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		after, err := client.ListTools(context.Background(), "dynamic")
		if err != nil {
			t.Fatal(err)
		}
		if len(after) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tool list remained stale after server change")
}

func TestDiscoveryRejectsInvalidToolName(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "invalid", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "valid"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				return &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "bad/name", InputSchema: map[string]any{"type": "object"}}}}, nil
			}
			return next(ctx, method, req)
		}
	})
	left, right := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := New()
	defer client.Close()
	if err := client.connect(context.Background(), "invalid", right); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(context.Background(), "invalid"); err == nil || !strings.Contains(err.Error(), "invalid tool name") {
		t.Fatalf("expected name error, got %v", err)
	}
}
