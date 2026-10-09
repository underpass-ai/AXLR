package engines

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeEngine is an MCP server speaking newline JSON over pipes, as an
// engine does over its stdio: echo answers, slow waits until released.
func fakeEngine(t *testing.T, release <-chan struct{}) (stdin io.WriteCloser, stdout io.ReadCloser) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "fake-engine", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo:" + args.Text}}, StructuredContent: map[string]any{"text": args.Text}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "slow"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "slow"}}}, nil, nil
	})
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	session, err := server.Connect(context.Background(), &mcp.IOTransport{Reader: inReader, Writer: outWriter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return inWriter, outReader
}

// muxClientSession connects an MCP client to the mux over an in-memory
// connection, as a console does over the socket.
func muxClientSession(t *testing.T, m *mux) *mcp.ClientSession {
	t.Helper()
	console, daemon := net.Pipe()
	go m.serve(daemon)
	client := mcp.NewClient(&mcp.Implementation{Name: "console", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.IOTransport{Reader: console, Writer: console}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestTwoConsolesShareOneEngine(t *testing.T) {
	release := make(chan struct{})
	stdin, stdout := fakeEngine(t, release)
	m, _, err := startMux(stdin, stdout, "test")
	if err != nil {
		t.Fatal(err)
	}
	first, second := muxClientSession(t, m), muxClientSession(t, m)
	if first.InitializeResult().ServerInfo.Name != "fake-engine" || second.InitializeResult().ServerInfo.Name != "fake-engine" {
		t.Fatal("initialize was not answered from the engine's")
	}
	// A slow call of one console does not hold the other's.
	slow := make(chan error, 1)
	go func() {
		_, err := first.CallTool(context.Background(), &mcp.CallToolParams{Name: "slow", Arguments: map[string]any{}})
		slow <- err
	}()
	var wg sync.WaitGroup
	for i, session := range []*mcp.ClientSession{first, second, second} {
		wg.Add(1)
		go func(i int, session *mcp.ClientSession) {
			defer wg.Done()
			text := strings.Repeat("x", i+1)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": text}})
			if err != nil || result.Content[0].(*mcp.TextContent).Text != "echo:"+text {
				t.Errorf("call %d: %+v %v", i, result, err)
			}
		}(i, session)
	}
	wg.Wait()
	tools, err := second.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("tools: %+v %v", tools, err)
	}
	select {
	case err := <-slow:
		t.Fatalf("the slow call returned early: %v", err)
	default:
	}
	close(release)
	if err := <-slow; err != nil {
		t.Fatal(err)
	}
}

// A console that goes away mid-call leaves the others working; the
// engine's late answer to it is dropped.
func TestAConsoleLeavingMidCallLeavesTheOthersWorking(t *testing.T) {
	release := make(chan struct{})
	stdin, stdout := fakeEngine(t, release)
	m, _, err := startMux(stdin, stdout, "test")
	if err != nil {
		t.Fatal(err)
	}
	leaving, staying := muxClientSession(t, m), muxClientSession(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _, _ = leaving.CallTool(ctx, &mcp.CallToolParams{Name: "slow", Arguments: map[string]any{}}) }()
	time.Sleep(50 * time.Millisecond)
	cancel() // sends notifications/cancelled through the mux
	_ = leaving.Close()
	close(release)
	if _, err := staying.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "ok"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for m.clientCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.clientCount() != 1 {
		t.Fatalf("clients = %d", m.clientCount())
	}
}

// The mux rewrites ids both ways, answers requests the engine makes of its
// client, routes cancellations and drops progress meant for one request.
func TestTheMuxRewritesIDsAndAnswersEngineRequests(t *testing.T) {
	engineIn, toMux := io.Pipe()    // the mux writes the engine's stdin
	fromMux, engineOut := io.Pipe() // the mux reads the engine's stdout
	engine := bufio.NewScanner(engineIn)
	readEngine := func() map[string]json.RawMessage {
		t.Helper()
		if !engine.Scan() {
			t.Fatal("engine input closed")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(engine.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}
	writeEngine := func(line string) {
		t.Helper()
		if _, err := io.WriteString(engineOut, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan *mux, 1)
	go func() {
		m, _, err := startMux(toMux, fromMux, "test")
		if err != nil {
			t.Error(err)
		}
		started <- m
	}()
	if init := readEngine(); string(init["method"]) != `"initialize"` {
		t.Fatalf("first message %s", init["method"])
	}
	writeEngine(`{"jsonrpc":"2.0","id":"axlr-engines-initialize","result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"e","version":"1"}}}`)
	if note := readEngine(); string(note["method"]) != `"notifications/initialized"` {
		t.Fatalf("second message %s", note["method"])
	}
	m := <-started
	console, daemon := net.Pipe()
	go m.serve(daemon)
	client := bufio.NewScanner(console)
	send := func(line string) {
		t.Helper()
		if _, err := io.WriteString(console, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	read := func() string {
		t.Helper()
		if !client.Scan() {
			t.Fatal("console connection closed")
		}
		return client.Text()
	}
	send(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}`)
	if got := read(); !strings.Contains(got, `"id":7`) || !strings.Contains(got, `"2024-11-05"`) {
		t.Fatalf("initialize answer %s", got)
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{"name":"x"}}`)
	call := readEngine()
	if string(call["id"]) != "1" {
		t.Fatalf("forwarded id %s", call["id"])
	}
	send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"a","reason":"stop"}}`)
	if cancelled := readEngine(); !strings.Contains(string(cancelled["params"]), `"requestId":1`) {
		t.Fatalf("cancellation %s", cancelled["params"])
	}
	writeEngine(`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"t","progress":1}}`)
	writeEngine(`{"jsonrpc":"2.0","id":99,"method":"roots/list"}`)
	if refused := readEngine(); string(refused["id"]) != "99" || refused["error"] == nil {
		t.Fatalf("engine request answer %v", refused)
	}
	writeEngine(`{"jsonrpc":"2.0","id":100,"method":"ping"}`)
	if pong := readEngine(); string(pong["id"]) != "100" || string(pong["result"]) != "{}" {
		t.Fatalf("ping answer %v", pong)
	}
	// Progress was dropped: the next thing the console reads is the
	// broadcast, then the answer under its own id. (net.Pipe has no buffer,
	// so each is read before the engine writes the next.)
	writeEngine(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`)
	if got := read(); !strings.Contains(got, "tools/list_changed") {
		t.Fatalf("broadcast %s", got)
	}
	writeEngine(`{"jsonrpc":"2.0","id":1,"result":{"content":[]}}`)
	if got := read(); !strings.Contains(got, `"id":"a"`) {
		t.Fatalf("answer %s", got)
	}
	// The engine going away closes the consoles.
	_ = engineOut.Close()
	if client.Scan() {
		t.Fatalf("console still open: %s", client.Text())
	}
}
