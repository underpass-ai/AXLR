package service

import (
	"encoding/json"
	"testing"
)

// The service has no Search or List verb (they would change the gRPC and
// OpenAPI inventories); create_tool_call reaches local_search and
// local_list by catalog name, validated against their schemas and held for
// approval like local_read.
func TestCreateToolCallReachesLocalSearchAndList(t *testing.T) {
	_, ts, client, _ := testServer(t)
	for key, tc := range []struct {
		body   string
		status int
	}{
		{`{"tool":"local_search","arguments":{"pattern":"needle","glob":"*.go"}}`, 202},
		{`{"tool":"local_list","arguments":{"recursive":true}}`, 202},
		{`{"tool":"local_search","arguments":{"pattern":"needle","context_lines":9}}`, 422},
	} {
		response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls", tc.body, "local-listing-key-"+string(rune('a'+key)))
		var created struct {
			ID     string `json:"call_id"`
			Status string `json:"status"`
		}
		_ = json.NewDecoder(response.Body).Decode(&created)
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("%s: %d", tc.body, response.StatusCode)
		}
		if tc.status == 202 && directCallStatus(t, client, ts.URL, created.ID) != "pending_approval" {
			t.Fatalf("%s was not held for approval", tc.body)
		}
	}
}
