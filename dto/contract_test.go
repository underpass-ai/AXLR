package dto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkerDTOJSONContract(t *testing.T) {
	raw := []byte(`{"protocol_version":1,"request_id":"r1","tool":"read","arguments":{"path":"a.txt"}}`)
	var request Request
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.ProtocolVersion != 1 || request.RequestID != "r1" || request.Tool != "read" || string(request.Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("request changed: %+v", request)
	}
	response := Response{ProtocolVersion: 1, RequestID: "r1", Tool: "read", Status: "completed", StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(0, 0).UTC(), Output: ReadOutput{Content: "ok", ReturnedBytes: 2}, Error: nil}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["output"]) != `{"content":"ok","start_offset_bytes":0,"returned_bytes":2,"next_offset_bytes":0,"truncated":false}` {
		t.Fatalf("output contract changed: %s", wire["output"])
	}
}
