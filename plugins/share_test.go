package plugins

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A registration reached through a shared process's socket is served from
// there; when sharing fails the manager launches the command itself.
func TestManagerUsesTheSharedSocketOrFallsBackToTheCommand(t *testing.T) {
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	registration, err := NewRegistration(Manifest{ID: "kmp", Command: command, Args: []string{"-test.run=^TestPluginHelper$"}, AllowAll: true}, []string{"AXLR_PLUGIN_HELPER=1"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "axp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "k.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer listener.Close()
	served := make(chan struct{}, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		served <- struct{}{}
		server := mcp.NewServer(&mcp.Implementation{Name: "shared", Version: "1"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "only_shared"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
		_ = server.Run(context.Background(), &mcp.IOTransport{Reader: conn, Writer: conn})
	}()
	for _, shared := range []bool{true, false} {
		manager, err := NewManager([]Registration{registration})
		if err != nil {
			t.Fatal(err)
		}
		manager.SetShare(func(context.Context, Registration) (string, error) {
			if shared {
				return socket, nil
			}
			return "", errors.New("no daemon")
		})
		tools, err := manager.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fromShared := len(tools) == 1 && tools[0].Ref.ToolName == "only_shared"
		if fromShared != shared {
			t.Fatalf("shared=%v: tools %+v", shared, tools)
		}
		_ = manager.Close()
	}
	select {
	case <-served:
	default:
		t.Fatal("the socket was never used")
	}
}
