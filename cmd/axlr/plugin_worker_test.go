package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/underpass-ai/AXLR/dto"
)

func workerManifest(t *testing.T, tools []string) string {
	t.Helper()
	command, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"manifest_version": 1, "id": "probe", "command": command, "args": []string{"-test.run=^TestWorkerPluginHelper$"}, "allow_tools": tools})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runPluginWorker(t *testing.T, manifest, request string, env ...string) (int, dto.Response, string) {
	t.Helper()
	args := []string{"--root", t.TempDir(), "--profile", "trusted-local", "--plugin", manifest}
	for _, item := range env {
		args = append(args, "--plugin-env", item)
	}
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(request), &stdout, &stderr)
	var response dto.Response
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("worker response: %s; stderr: %s; err: %v", stdout.String(), stderr.String(), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("worker emitted extra data: %s", stdout.String())
	}
	return code, response, stderr.String()
}

func TestWorkerListsAndCallsPluginTools(t *testing.T) {
	manifest := workerManifest(t, []string{"echo", "env"})
	listRequest := `{"protocol_version":1,"request_id":"p1","tool":"plugins.list","arguments":{}}`
	code, response, stderr := runPluginWorker(t, manifest, listRequest, "probe:AXLR_WORKER_HELPER=1")
	if code != 0 || response.Status != "completed" || stderr != "" {
		t.Fatalf("list: %d, %+v, %q", code, response, stderr)
	}
	listJSON, _ := json.Marshal(response.Output)
	if !bytes.Contains(listJSON, []byte(`"tool_name":"echo"`)) || bytes.Contains(listJSON, []byte(`"tool_name":"hidden"`)) {
		t.Fatalf("list output: %s", listJSON)
	}
	callRequest := `{"protocol_version":1,"request_id":"p2","tool":"plugins.call","arguments":{"plugin_id":"probe","tool_name":"echo","arguments":{"text":"hello"}}}`
	code, response, stderr = runPluginWorker(t, manifest, callRequest, "probe:AXLR_WORKER_HELPER=1")
	callJSON, _ := json.Marshal(response.Output)
	if code != 0 || response.Status != "completed" || !bytes.Contains(callJSON, []byte("hello")) || stderr != "" {
		t.Fatalf("call: %d, %+v, %q", code, response, stderr)
	}
}

func TestWorkerPluginEnvironmentIsExplicitAndLocalIsLazy(t *testing.T) {
	t.Setenv("AXLR_INHERITED_SECRET", "must-not-leak")
	manifest := workerManifest(t, []string{"env"})
	startLog := filepath.Join(t.TempDir(), "starts")
	local := `{"protocol_version":1,"request_id":"l1","tool":"read","arguments":{"path":"missing"}}`
	_, _, _ = runPluginWorker(t, manifest, local, "probe:AXLR_WORKER_HELPER=1", "probe:AXLR_START_LOG="+startLog)
	if _, err := os.Stat(startLog); !os.IsNotExist(err) {
		t.Fatal("local tool launched plugin")
	}
	request := `{"protocol_version":1,"request_id":"p3","tool":"plugins.call","arguments":{"plugin_id":"probe","tool_name":"env","arguments":{}}}`
	code, response, _ := runPluginWorker(t, manifest, request, "probe:AXLR_WORKER_HELPER=1", "probe:AXLR_EXPLICIT=present", "probe:AXLR_START_LOG="+startLog)
	encoded, _ := json.Marshal(response.Output)
	if code != 0 || response.Status != "completed" || !bytes.Contains(encoded, []byte("present:false")) {
		t.Fatalf("plugin env: %d, %+v", code, response)
	}
	if _, err := os.Stat(startLog); err != nil {
		t.Fatalf("plugin did not start: %v", err)
	}
}

func TestWorkerRejectsBadManifestAndUnknownPluginEnv(t *testing.T) {
	manifest := workerManifest(t, []string{"echo"})
	request := `{"protocol_version":1,"request_id":"p4","tool":"plugins.list","arguments":{}}`
	for _, extra := range [][]string{{"--plugin-env", "missing:KEY=VALUE"}, {"--plugin", manifest}} {
		args := append([]string{"--root", t.TempDir(), "--profile", "trusted-local", "--plugin", manifest}, extra...)
		var out, stderr bytes.Buffer
		if code := run(args, strings.NewReader(request), &out, &stderr); code != 1 || out.Len() != 0 {
			t.Fatalf("bad config accepted: %d, %s, %s", code, out.String(), stderr.String())
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"manifest_version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{"--root", t.TempDir(), "--profile", "trusted-local", "--plugin", bad}, strings.NewReader(request), &out, &stderr); code != 1 || out.Len() != 0 {
		t.Fatalf("bad manifest accepted: %d, %s", code, out.String())
	}
}

func TestWorkerPluginHelper(t *testing.T) {
	if os.Getenv("AXLR_WORKER_HELPER") != "1" {
		return
	}
	if path := os.Getenv("AXLR_START_LOG"); path != "" {
		_ = os.WriteFile(path, []byte("started\n"), 0600)
	}
	_, _ = os.Stderr.WriteString("plugin diagnostic on stderr\n")
	server := mcp.NewServer(&mcp.Implementation{Name: "worker-helper", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Text}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "env"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		text := os.Getenv("AXLR_EXPLICIT") + ":" + strconv.FormatBool(os.Getenv("AXLR_INHERITED_SECRET") != "")
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "hidden"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "wait"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		if path := os.Getenv("AXLR_CALL_LOG"); path != "" {
			_ = os.WriteFile(path, []byte("called\n"), 0600)
		}
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestWorkerProcess(t *testing.T) {
	if os.Getenv("AXLR_WORKER_PROCESS") != "1" {
		return
	}
	code := run([]string{"--root", os.Getenv("AXLR_ROOT"), "--profile", "trusted-local", "--plugin", os.Getenv("AXLR_MANIFEST"), "--plugin-env", "probe:AXLR_WORKER_HELPER=1", "--plugin-env", "probe:AXLR_CALL_LOG=" + os.Getenv("AXLR_CALL_LOG")}, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(code)
}

func TestWorkerSignalCancelsPluginCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM")
	}
	manifest := workerManifest(t, []string{"wait"})
	callLog := filepath.Join(t.TempDir(), "calls")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerProcess$")
	cmd.Env = append(os.Environ(), "AXLR_WORKER_PROCESS=1", "AXLR_ROOT="+t.TempDir(), "AXLR_MANIFEST="+manifest, "AXLR_CALL_LOG="+callLog)
	cmd.Stdin = strings.NewReader(`{"protocol_version":1,"request_id":"cancel","tool":"plugins.call","arguments":{"plugin_id":"probe","tool_name":"wait","arguments":{}}}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(callLog); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("plugin call never started: %s", stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("worker terminated abnormally: %v; %s", err, stderr.String())
	}
	var response dto.Response
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil || response.Status != "cancelled" {
		t.Fatalf("cancellation: %s, %v; stderr: %s", stdout.String(), err, stderr.String())
	}
}
