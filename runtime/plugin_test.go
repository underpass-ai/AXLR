package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

type pluginPortStub struct {
	listResult []domain.PluginTool
	callResult domain.PluginResult
	err        error
}

func (p pluginPortStub) List(context.Context) ([]domain.PluginTool, error) {
	return p.listResult, p.err
}
func (p pluginPortStub) Call(context.Context, domain.PluginCall) (domain.PluginResult, error) {
	return p.callResult, p.err
}

func pluginRequest(tool, args string) dto.Request {
	return dto.Request{ProtocolVersion: 1, RequestID: "plugin-1", Tool: tool, Arguments: json.RawMessage(args)}
}

func TestPluginListProducesTypedOutput(t *testing.T) {
	schema, _ := domain.NewJSONValue([]byte(`{"type":"object"}`))
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Plugins: pluginPortStub{listResult: []domain.PluginTool{{Ref: domain.PluginRef{PluginID: "search", ToolName: "find"}, Description: "Find", InputSchema: schema}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	request, err := Decode(strings.NewReader(`{"protocol_version":1,"request_id":"plugin-1","tool":"plugins.list","arguments":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	response := e.Execute(context.Background(), request)
	if response.Status != "completed" {
		t.Fatalf("response: %+v", response)
	}
	output, ok := response.Output.(dto.PluginListOutput)
	if !ok || len(output.Tools) != 1 || output.Tools[0].ToolName != "find" || string(output.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("output: %#v", response.Output)
	}
	if _, err := Decode(strings.NewReader(`{"protocol_version":1,"request_id":"plugin-1","tool":"plugins.list","arguments":{"extra":true}}`)); err == nil {
		t.Fatal("list accepted extra arguments")
	}
}

func TestPluginCallRetainsToolLevelError(t *testing.T) {
	block, _ := domain.NewJSONValue([]byte(`{"type":"text","text":"failed"}`))
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Plugins: pluginPortStub{callResult: domain.PluginResult{Content: []domain.JSONValue{block}, IsError: true}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	request := pluginRequest("plugins.call", `{"plugin_id":"search","tool_name":"find","arguments":{"q":"x"}}`)
	response := e.Execute(context.Background(), request)
	if response.Status != "completed" || !response.Output.(dto.PluginCallOutput).IsError {
		t.Fatalf("tool-level error became failure: %+v", response)
	}
	encoded, err := MarshalResponse(response)
	if err != nil || !strings.Contains(string(encoded), `"tool":"plugins.call"`) {
		t.Fatalf("response encoding: %s, %v", encoded, err)
	}
	for _, args := range []string{`{"plugin_id":"bad.id","tool_name":"find","arguments":{}}`, `{"plugin_id":"search","tool_name":"bad/name","arguments":{}}`, `{"plugin_id":"search","tool_name":"find","arguments":[]}`} {
		if _, err := (RequestMapper{}).Map(pluginRequest("plugins.call", args)); err == nil {
			t.Errorf("accepted invalid call: %s", args)
		}
	}
}

func TestPluginFailuresKeepTheirStatus(t *testing.T) {
	for name, input := range map[string]struct {
		err  error
		want string
	}{
		"unknown":   {domain.Reject("unknown_plugin", "missing"), "rejected"},
		"protocol":  {errors.New("broken pipe"), "failed"},
		"cancelled": {context.Canceled, "cancelled"},
		"timeout":   {context.DeadlineExceeded, "timed_out"},
	} {
		t.Run(name, func(t *testing.T) {
			e, err := newTestExecutor(t, Config{Root: t.TempDir(), Plugins: pluginPortStub{err: input.err}})
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			response := e.Execute(context.Background(), pluginRequest("plugins.call", `{"plugin_id":"search","tool_name":"find","arguments":{}}`))
			if response.Status != input.want {
				t.Fatalf("want %s, got %+v", input.want, response)
			}
		})
	}
}

func TestLocalReadNeverTouchesPluginPort(t *testing.T) {
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Plugins: pluginPortStub{err: errors.New("plugin touched")}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	response := e.Execute(context.Background(), pluginRequest("read", `{"path":"missing"}`))
	if response.Error != nil && response.Error.Message == "plugin touched" {
		t.Fatalf("local read used plugin port: %+v", response)
	}
}
