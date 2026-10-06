package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/domain"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type parityCatalog struct {
	tools []domain.AvailableTool
	reads atomic.Int32
	fail  atomic.Bool
}

func (c *parityCatalog) Snapshot(context.Context) ([]domain.AvailableTool, error) {
	c.reads.Add(1)
	if c.fail.Load() {
		return nil, io.ErrUnexpectedEOF
	}
	return append([]domain.AvailableTool(nil), c.tools...), nil
}

type countingTool struct{ calls atomic.Int32 }

func (t *countingTool) Execute(_ context.Context, identity domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
	t.calls.Add(1)
	return domain.ToolOutcome{Content: root.Text(identity.Plugin.PluginID.String() + "/" + identity.Plugin.ToolName.String())}, nil
}

type parityClients struct {
	s         *Server
	http      *http.Client
	unmapped  *http.Client
	url       string
	grpc      *grpc.ClientConn
	mcp       *mcp.ClientSession
	grpcAddr  string
	progress  chan *mcp.ProgressNotificationParams
	catalog   *parityCatalog
	execution *countingTool
}

func newParityClients(t *testing.T) *parityClients {
	t.Helper()
	s, ts, client, unmapped := testServer(t)
	tools, err := (axlr.ToolCatalog{}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{},"additionalProperties":false}`))
	for _, id := range []string{"alpha", "beta"} {
		identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: root.PluginID(id), ToolName: "same_name"})
		tools = append(tools, domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{Name: root.ToolName("alias_" + id), Parameters: schema}})
	}
	catalog := &parityCatalog{tools: tools}
	execution := &countingTool{}
	s.deps.Catalog, s.deps.Tools = catalog, execution
	server, err := s.grpcServer()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	t.Cleanup(func() { listener.Close() })
	tlsConfig := client.Transport.(*http.Transport).TLSClientConfig.Clone()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	progress := make(chan *mcp.ProgressNotificationParams, 10)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "parity-test", Version: "1"}, &mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) { progress <- req.Params }})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: client, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return &parityClients{s, client, unmapped, ts.URL, conn, session, listener.Addr().String(), progress, catalog, execution}
}

func decodeResult(t *testing.T, data []byte) operationResult {
	t.Helper()
	var result operationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid operation result: %s: %v", data, err)
	}
	return result
}

func grpcResult(t *testing.T, result *wrapperspb.BytesValue, err error) operationResult {
	t.Helper()
	if err == nil {
		return decodeResult(t, result.Value)
	}
	for _, detail := range status.Convert(err).Details() {
		if detail, ok := detail.(*errdetails.ErrorInfo); ok {
			return decodeResult(t, []byte(detail.Metadata["result_json"]))
		}
	}
	t.Fatalf("gRPC error lost service envelope: %v", err)
	return operationResult{}
}

func (c *parityClients) call(t *testing.T, transport, name, input string) operationResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch transport {
	case "http":
		response := apiRequest(t, c.http, "POST", c.url+"/v1/operations/"+name, input, "")
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return decodeResult(t, data)
	case "grpc":
		if name == "StreamEvents" {
			stream, err := c.grpc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/"+grpcServiceName+"/"+name)
			if err != nil {
				t.Fatal(err)
			}
			if err := stream.SendMsg(wrapperspb.Bytes([]byte(input))); err != nil {
				t.Fatal(err)
			}
			_ = stream.CloseSend()
			var final *wrapperspb.BytesValue
			for {
				value := new(wrapperspb.BytesValue)
				err := stream.RecvMsg(value)
				if err == io.EOF {
					if final == nil {
						t.Fatal("stream returned no final envelope")
					}
					return decodeResult(t, final.Value)
				}
				if err != nil {
					return grpcResult(t, nil, err)
				}
				if strings.Contains(string(value.Value), `"status_code"`) {
					final = value
				}
			}
		}
		output := new(wrapperspb.BytesValue)
		err := c.grpc.Invoke(ctx, "/"+grpcServiceName+"/"+name, wrapperspb.Bytes([]byte(input)), output)
		return grpcResult(t, output, err)
	case "mcp":
		op, _ := findOperation(name)
		output, err := c.mcp.CallTool(ctx, &mcp.CallToolParams{Name: op.Tool, Arguments: json.RawMessage(input)})
		if err != nil {
			t.Fatal(err)
		}
		result := decodeResult(t, []byte(output.Content[0].(*mcp.TextContent).Text))
		if output.IsError != (result.StatusCode >= 400) {
			t.Fatal("MCP error flag disagrees with service status")
		}
		return result
	}
	t.Fatal("unknown transport")
	return operationResult{}
}

func seedParitySession(t *testing.T, s *Server, pending bool) string {
	t.Helper()
	id, _ := newID()
	session, _ := domain.NewSession(domain.SessionID(id), domain.Workspace(s.Config.Workspace), "test/model")
	if pending {
		tools, _ := s.deps.Catalog.Snapshot(context.Background())
		if err := session.BeginTurn("read a file", tools); err != nil {
			t.Fatal(err)
		}
		args, _ := root.NewJSONObject([]byte(`{"path":"sample.txt"}`))
		call := root.ToolCall{ID: "pending-call", Name: "local_read", Arguments: args}
		if err := session.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}}); err != nil {
			t.Fatal(err)
		}
	}
	session.SetServiceMetadata("alice", 0, "")
	if err := s.sessions.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedParityCall(t *testing.T, s *Server) string {
	t.Helper()
	id, _ := newID()
	identity, _ := domain.NewLocalToolIdentity("read")
	if err := s.calls.Save(toolCall{ID: id, Owner: "alice", Tool: "local_read", Identity: identity, Args: json.RawMessage(`{"path":"sample.txt"}`), Status: "pending_approval", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEveryServiceVerbWorksAcrossHTTPGRPCAndMCP(t *testing.T) {
	c := newParityClients(t)
	for _, op := range operations() {
		for _, transport := range []string{"http", "grpc", "mcp"} {
			t.Run(op.Name+"/"+transport, func(t *testing.T) {
				input := map[string]any{"request_id": "parity-request"}
				key, _ := newID()
				input["idempotency_key"] = key
				expected := 200
				switch op.Name {
				case "Read", "Write", "Edit", "Exec", "CreateToolCall":
					expected = 202
					args := map[string]any{"path": "sample.txt"}
					if op.Name == "Write" {
						args["content"], args["mode"] = "hello", "create"
					} else if op.Name == "Edit" {
						args["old_text"], args["new_text"] = "old", "new"
					} else if op.Name == "Exec" {
						args = map[string]any{"program": "echo", "args": []string{"hello"}}
					}
					input["arguments"] = args
					if op.Name == "CreateToolCall" {
						input["tool"] = "local_read"
					}
				case "CallPlugin":
					expected = 202
					input["plugin_id"], input["tool_name"], input["arguments"] = "beta", "same_name", map[string]any{}
				case "CreateSession":
					expected = 201
					input["model"] = "test/model"
				case "GetSession", "StartTurn", "StreamEvents", "DecideSessionTool", "CancelSessionTurn":
					id := seedParitySession(t, c.s, op.Name == "DecideSessionTool" || op.Name == "CancelSessionTurn")
					input["session_id"] = id
					if op.Name == "StartTurn" {
						input["prompt"], input["expected_revision"], expected = "hello", 1, 202
					} else if op.Name == "DecideSessionTool" {
						input["call_id"], input["decision"], input["expected_revision"], expected = "pending-call", "deny", 1, 202
					} else if op.Name == "CancelSessionTurn" {
						input["expected_revision"], expected = 1, 202
					} else if op.Name == "StreamEvents" {
						_, _ = c.s.events.Append(id, "op", "text.delta", map[string]string{"text": "hello"})
					}
				case "GetToolCall", "DecideToolCall":
					input["call_id"] = seedParityCall(t, c.s)
					if op.Name == "DecideToolCall" {
						input["decision"], input["expected_revision"], expected = "deny", 1, 202
					}
				case "Liveness", "Readiness":
					expected = 204
				}
				data, _ := json.Marshal(input)
				result := c.call(t, transport, op.Name, string(data))
				if result.StatusCode != expected || result.RequestID != "parity-request" {
					t.Fatalf("%s: %+v", op.Name, result)
				}
			})
		}
	}
	if c.execution.calls.Load() != 0 {
		t.Fatal("transport aliases bypassed manual approval")
	}
}

func TestCrossTransportReplayApprovesExactPluginOnce(t *testing.T) {
	c := newParityClients(t)
	input := `{"plugin_id":"beta","tool_name":"same_name","arguments":{},"idempotency_key":"shared-call-key-1234","request_id":"shared"}`
	created := c.call(t, "http", "CallPlugin", input)
	reads := c.catalog.reads.Load()
	c.catalog.fail.Store(true)
	for _, transport := range []string{"grpc", "mcp"} {
		if got := c.call(t, transport, "CallPlugin", input); !reflect.DeepEqual(created, got) {
			t.Fatalf("replay changed identity/result: %s: %+v %+v", transport, created, got)
		}
	}
	if c.catalog.reads.Load() != reads {
		t.Fatal("plugin replay required discovery")
	}
	var body struct {
		CallID string `json:"call_id"`
	}
	_ = json.Unmarshal(created.Body, &body)
	input = `{"call_id":"` + body.CallID + `","decision":"approve","expected_revision":1,"idempotency_key":"shared-decision-1234","request_id":"decision"}`
	for _, transport := range []string{"http", "grpc", "mcp"} {
		if result := c.call(t, transport, "DecideToolCall", input); result.StatusCode != 202 {
			t.Fatal(result)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		call, _ := c.s.calls.Load(body.CallID)
		if call.Status == "completed" {
			if call.Result.Content != "beta/same_name" || c.execution.calls.Load() != 1 {
				t.Fatalf("wrong or repeated effect: %+v, executions=%d", call, c.execution.calls.Load())
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approved call did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	changed := `{"plugin_id":"alpha","tool_name":"same_name","arguments":{},"idempotency_key":"shared-call-key-1234"}`
	if got := c.call(t, "mcp", "CallPlugin", changed); got.StatusCode != 409 {
		t.Fatalf("conflicting identity reused key: %+v", got)
	}
}

func TestEventReplayIsIdenticalAndMCPPublishesLiveProgress(t *testing.T) {
	c := newParityClients(t)
	id := seedParitySession(t, c.s, false)
	for _, text := range []string{"one", "two", "three"} {
		_, _ = c.s.events.Append(id, "op", "text.delta", map[string]string{"text": text})
	}
	input := `{"session_id":"` + id + `","after":1,"max_events":2,"request_id":"events"}`
	baseline := c.call(t, "http", "StreamEvents", input)
	for _, transport := range []string{"grpc", "mcp"} {
		if result := c.call(t, transport, "StreamEvents", input); !reflect.DeepEqual(result, baseline) {
			t.Fatalf("event replay differs: %+v %+v", baseline, result)
		}
	}
	params := &mcp.CallToolParams{Name: "axlr_stream_events", Arguments: map[string]any{"session_id": id, "after": 3, "max_events": 1, "wait_ms": 2000}}
	params.SetProgressToken("live-progress")
	done := make(chan error, 1)
	go func() { _, err := c.mcp.CallTool(context.Background(), params); done <- err }()
	_, _ = c.s.events.Append(id, "op", "text.delta", map[string]string{"text": "live"})
	select {
	case progress := <-c.progress:
		if progress.ProgressToken != "live-progress" || progress.Progress != 4 || !strings.Contains(progress.Message, "live") {
			t.Fatal(progress)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("MCP buffered events without live progress")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTransportErrorsRolesAndExactUint64Revisions(t *testing.T) {
	c := newParityClients(t)
	id := seedParitySession(t, c.s, false)
	session, _ := c.s.sessions.Load(context.Background(), domain.SessionID(id))
	session.SetServiceMetadata("alice", 9007199254740993, "")
	// Write through the base store to seed a revision beyond float64 precision.
	if err := c.s.base.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	for _, transport := range []string{"http", "grpc", "mcp"} {
		got := c.call(t, transport, "GetSession", `{"session_id":"`+id+`"}`)
		if !strings.Contains(string(got.Body), `"revision":9007199254740993`) {
			t.Fatalf("revision rounded by %s: %s", transport, got.Body)
		}
		for _, tc := range []struct {
			op, input string
			status    int
		}{
			{"GetSession", `{"session_id":"../escape"}`, 400},
			{"GetSession", `{"session_id":"` + id + `","extra":true}`, 400},
			{"GetSession", `{"session_id":"` + id + `","idempotency_key":"short"}`, 400},
			{"Read", `{"arguments":{"path":"a"},"idempotency_key":"short"}`, 400},
			{"Read", `{"arguments":{"path":"a","extra":true},"idempotency_key":"valid-key-123456789"}`, 422},
			{"StreamEvents", `{"session_id":"` + id + `","after":99}`, 400},
			{"StreamEvents", `{"session_id":"` + id + `","wait_ms":30001}`, 400},
			{"StreamEvents", `{"session_id":"` + id + `","max_events": 0 }`, 400},
			{"StreamEvents", `{"session_id":"` + id + `","after":18446744073709551616}`, 400},
		} {
			if got := c.call(t, transport, tc.op, tc.input); got.StatusCode != tc.status {
				t.Fatalf("%s/%s: %+v", transport, tc.op, got)
			}
		}
	}
	for _, p := range c.s.principals {
		delete(p.Roles, "tool_operator")
	}
	for _, transport := range []string{"http", "grpc", "mcp"} {
		if got := c.call(t, transport, "ListTools", `{}`); got.StatusCode != 403 {
			t.Fatalf("role bypass on %s: %+v", transport, got)
		}
	}
	response := apiRequest(t, c.unmapped, "POST", c.url+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "")
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("unmapped MCP certificate admitted")
	}
	conn, err := grpc.NewClient(c.grpcAddr, grpc.WithTransportCredentials(credentials.NewTLS(c.unmapped.Transport.(*http.Transport).TLSClientConfig.Clone())))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	output := new(wrapperspb.BytesValue)
	err = conn.Invoke(context.Background(), "/"+grpcServiceName+"/ListTools", wrapperspb.Bytes([]byte(`{}`)), output)
	if grpcResult(t, output, err).StatusCode != 403 {
		t.Fatal("unmapped gRPC certificate admitted")
	}
}

func TestProtoAndMCPInventoriesHaveNoMissingOrExtraVerbs(t *testing.T) {
	c := newParityClients(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "api", "proto", "underpass", "axlr", "v1", "axlr.proto"))
	if err != nil {
		t.Fatal(err)
	}
	rpcNames := regexp.MustCompile(`rpc\s+(\w+)\(`).FindAllStringSubmatch(string(data), -1)
	result, err := c.mcp.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpcNames) != 18 || len(result.Tools) != 18 {
		t.Fatalf("inventory drift: RPC=%d MCP=%d", len(rpcNames), len(result.Tools))
	}
	for _, op := range operations() {
		foundRPC, foundMCP := false, false
		for _, rpc := range rpcNames {
			foundRPC = foundRPC || rpc[1] == op.Name
		}
		for _, tool := range result.Tools {
			if tool.Name == op.Tool {
				foundMCP = true
				got, _ := json.Marshal(tool.InputSchema)
				want, _ := json.Marshal(op.schema())
				// The SDK decodes schema metadata into interface values with
				// float64 numbers. Compare using that same representation.
				var decoded any
				if err := json.Unmarshal(want, &decoded); err != nil {
					t.Fatal(err)
				}
				want, _ = json.Marshal(decoded)
				if string(got) != string(want) {
					t.Fatalf("schema drift: %s", op.Name)
				}
			}
		}
		if !foundRPC || !foundMCP {
			t.Fatal("missing verb", op.Name)
		}
	}
}

func TestGRPCRejectsMissingClientCertificateAndStreamsImmediately(t *testing.T) {
	c := newParityClients(t)
	config := c.http.Transport.(*http.Transport).TLSClientConfig.Clone()
	config.Certificates = nil
	conn, err := grpc.NewClient(c.grpcAddr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = conn.Invoke(ctx, "/"+grpcServiceName+"/ListTools", wrapperspb.Bytes([]byte(`{}`)), new(wrapperspb.BytesValue))
	if err == nil || status.Code(err) != codes.Unavailable && status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("missing certificate was not rejected by transport: %v", err)
	}
	id := seedParitySession(t, c.s, false)
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	stream, err := c.grpc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/"+grpcServiceName+"/StreamEvents")
	if err != nil {
		t.Fatal(err)
	}
	input := `{"session_id":"` + id + `","wait_ms":2000,"max_events":1}`
	if err := stream.SendMsg(wrapperspb.Bytes([]byte(input))); err != nil {
		t.Fatal(err)
	}
	_ = stream.CloseSend()
	_, _ = c.s.events.Append(id, "op", "text.delta", map[string]string{"text": "live-frame"})
	frame := new(wrapperspb.BytesValue)
	if err := stream.RecvMsg(frame); err != nil || !strings.Contains(string(frame.Value), `"kind":"event"`) || !strings.Contains(string(frame.Value), "live-frame") {
		t.Fatalf("missing live event frame: %s: %v", frame.Value, err)
	}
	if err := stream.RecvMsg(frame); err != nil || decodeResult(t, frame.Value).StatusCode != 200 {
		t.Fatalf("missing final cursor/page: %v", err)
	}
}

func TestSessionOwnershipAndStreamCancellationRemainIndependent(t *testing.T) {
	c := newParityClients(t)
	id := seedParitySession(t, c.s, false)
	session, _ := c.s.sessions.Load(context.Background(), domain.SessionID(id))
	session.SetServiceMetadata("another-principal", 1, "")
	if err := c.s.base.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	for _, transport := range []string{"http", "grpc", "mcp"} {
		for _, verb := range []string{"GetSession", "StreamEvents"} {
			if got := c.call(t, transport, verb, `{"session_id":"`+id+`"}`); got.StatusCode != 403 {
				t.Fatalf("ownership bypass: %s/%s: %+v", transport, verb, got)
			}
		}
	}
	id = seedParitySession(t, c.s, false)
	var cancelled atomic.Bool
	c.s.registerOperation(id, func() { cancelled.Store(true) })
	defer c.s.finishOperation(id)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.mcp.CallTool(ctx, &mcp.CallToolParams{Name: "axlr_stream_events", Arguments: map[string]any{"session_id": id, "wait_ms": 30000}})
		done <- err
	}()
	stop()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled stream unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("stream cancellation did not propagate")
	}
	if cancelled.Load() {
		t.Fatal("closing the event stream cancelled the model turn")
	}
	if got := c.call(t, "grpc", "CancelSessionTurn", `{"session_id":"`+id+`","expected_revision":1}`); got.StatusCode != 202 || !cancelled.Load() {
		t.Fatalf("explicit cancellation did not reach the turn: %+v", got)
	}
}

func TestConfiguredGRPCListenerBindsAndStopsWithService(t *testing.T) {
	c := newParityClients(t)
	c.s.Config.GRPCListen = c.grpcAddr
	if err := c.s.Serve(context.Background()); err == nil {
		t.Fatal("occupied gRPC listener was accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.s.Config.GRPCListen = listener.Addr().String()
	listener.Close()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() { done <- c.s.Serve(ctx) }()
	config := c.http.Transport.(*http.Transport).TLSClientConfig.Clone()
	if config.MinVersion != tls.VersionTLS13 {
		t.Fatal("test did not use the service TLS policy")
	}
	conn, err := grpc.NewClient(c.s.Config.GRPCListen, grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output := new(wrapperspb.BytesValue)
	err = conn.Invoke(callCtx, "/"+grpcServiceName+"/ListTools", wrapperspb.Bytes([]byte(`{}`)), output, grpc.WaitForReady(true))
	if err != nil || decodeResult(t, output.Value).StatusCode != 200 {
		t.Fatalf("configured listener did not serve: %v", err)
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configured listeners did not stop")
	}
}
