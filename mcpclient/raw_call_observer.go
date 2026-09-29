package mcpclient

import (
	"encoding/json"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// rawCallObserver preserves structuredContent before the SDK decodes numbers into float64.
type rawCallObserver struct {
	callMu sync.Mutex
	mu     sync.Mutex
	id     jsonrpc.ID
	active bool
	raw    json.RawMessage
}

func (o *rawCallObserver) begin() {
	o.callMu.Lock()
	o.mu.Lock()
	o.active = false
	o.raw = nil
	o.mu.Unlock()
}

func (o *rawCallObserver) sent(id jsonrpc.ID) {
	o.mu.Lock()
	o.id = id
	o.active = true
	o.raw = nil
	o.mu.Unlock()
}

func (o *rawCallObserver) received(response *jsonrpc.Response) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.active || response.ID != o.id {
		return
	}
	var result struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal(response.Result, &result) == nil {
		o.raw = append(json.RawMessage(nil), result.StructuredContent...)
	}
}

func (o *rawCallObserver) end() json.RawMessage {
	o.mu.Lock()
	raw := append(json.RawMessage(nil), o.raw...)
	o.active = false
	o.raw = nil
	o.mu.Unlock()
	o.callMu.Unlock()
	return raw
}
