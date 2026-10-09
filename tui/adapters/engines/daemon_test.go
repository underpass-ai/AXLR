//go:build unix

package engines

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/mcpclient"
	"github.com/underpass-ai/AXLR/plugins"
)

// TestMain lets the test binary play the engine and the daemon the
// supervisor starts, as axlr-tui does with --engines-serve.
func TestMain(m *testing.M) {
	if os.Getenv("AXLR_FAKE_ENGINE") == "1" {
		server := mcp.NewServer(&mcp.Implementation{Name: "fake-engine", Version: "1.0.0"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
			Text string `json:"text"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo:" + args.Text}}, StructuredContent: map[string]any{"pid": os.Getpid()}}, nil, nil
		})
		_ = server.Run(context.Background(), &mcp.StdioTransport{})
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--engines-serve" {
		if err := Serve(context.Background(), os.Args[2], os.Stdin, "test", time.Second); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// shortDir is a private directory with a short path: a socket path is
// limited to about 104 bytes.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "axe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "engines")
}

func fakeSpec(extra ...string) Spec {
	return Spec{Command: os.Args[0], Env: append([]string{"AXLR_FAKE_ENGINE=1"}, extra...)}
}

func enginePID(t *testing.T, client *mcpclient.Client, name mcpclient.ServerName) float64 {
	t.Helper()
	result, err := client.Call(context.Background(), mcpclient.ToolRef{Server: name, Name: "echo"}, map[string]any{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	var structured struct{ PID float64 }
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil || structured.PID == 0 || !strings.Contains(string(result.Content[0]), "echo:hi") {
		t.Fatalf("result %+v: %v", result, err)
	}
	return structured.PID
}

func TestConsolesShareOneEngineProcessPerSpec(t *testing.T) {
	ctx := context.Background()
	supervisor := Supervisor{Dir: shortDir(t), Executable: os.Args[0], Version: "test"}
	first, err := supervisor.Socket(ctx, fakeSpec())
	if err != nil {
		t.Fatal(err)
	}
	again, err := supervisor.Socket(ctx, fakeSpec())
	if err != nil || again != first {
		t.Fatalf("second socket %q, %v", again, err)
	}
	if info, err := os.Stat(first); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v, %v", info, err)
	}
	consoles := []*mcpclient.Client{mcpclient.New(), mcpclient.New()}
	var pids []float64
	for _, console := range consoles {
		if err := console.Connect(ctx, mcpclient.Server{Name: "kmp", Socket: first}); err != nil {
			t.Fatal(err)
		}
		if tools, err := console.ListTools(ctx, "kmp"); err != nil || len(tools) != 1 {
			t.Fatalf("tools %+v, %v", tools, err)
		}
		pids = append(pids, enginePID(t, console, "kmp"))
	}
	if pids[0] != pids[1] {
		t.Fatalf("two engine processes: %v", pids)
	}
	// Another environment is another engine: a store or a MADE host id
	// never meets another's.
	other, err := supervisor.Socket(ctx, fakeSpec("KMP_MCP_DATA_DIR=/elsewhere"))
	if err != nil || other == first {
		t.Fatalf("other spec socket %q, %v", other, err)
	}
	for _, console := range consoles {
		_ = console.Close()
	}
	// Without consoles the daemons exit after their idle time (1 s here).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(first); os.IsNotExist(err) {
			if _, err := os.Stat(other); os.IsNotExist(err) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the daemons did not exit when idle")
}

func TestTheSocketDirectoryMustBePrivate(t *testing.T) {
	dir := shortDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Supervisor{Dir: dir, Executable: os.Args[0]}.Socket(context.Background(), fakeSpec())
	if err == nil || !strings.Contains(err.Error(), "private to this user") {
		t.Fatalf("shared directory accepted: %v", err)
	}
	if _, err := (Supervisor{Dir: "relative", Executable: os.Args[0]}).Socket(context.Background(), fakeSpec()); err == nil {
		t.Fatal("relative directory accepted")
	}
	if _, err := (Supervisor{Dir: dir, Executable: os.Args[0]}).Socket(context.Background(), Spec{}); err == nil {
		t.Fatal("empty spec accepted")
	}
}

// The plugin manager reaches a shared engine through its socket, and
// starts the engine itself when sharing fails.
func TestThePluginManagerFallsBackToItsOwnEngine(t *testing.T) {
	ctx := context.Background()
	supervisor := Supervisor{Dir: shortDir(t), Executable: os.Args[0], Version: "test"}
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: os.Args[0], AllowAll: true}, []string{"AXLR_FAKE_ENGINE=1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []bool{true, false} {
		manager, err := plugins.NewManager([]plugins.Registration{registration})
		if err != nil {
			t.Fatal(err)
		}
		used := false
		manager.SetShare(func(ctx context.Context, r plugins.Registration) (string, error) {
			used = true
			if !shared {
				return "", os.ErrNotExist
			}
			return supervisor.Socket(ctx, Spec{Command: r.Manifest.Command, Args: r.Manifest.Args, Env: r.Env})
		})
		tools, err := manager.List(ctx)
		if err != nil || len(tools) != 1 || !used {
			t.Fatalf("shared=%v: tools %+v, %v, share used %v", shared, tools, err, used)
		}
		args, _ := domain.NewJSONObject([]byte(`{"text":"x"}`))
		if result, err := manager.Call(ctx, domain.PluginCall{Ref: domain.PluginRef{PluginID: "kmp", ToolName: "echo"}, Arguments: args}); err != nil || result.IsError {
			t.Fatalf("shared=%v: call %+v, %v", shared, result, err)
		}
		_ = manager.Close()
	}
}
