package contract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const contractPolicy = `{"policy": {"level": "observe", "tools": {"allow": [{"name": "observability.query_logs"}, {"name": "local_read", "arguments": {"path": "logs/**"}}], "deny": [{"name": "local_exec"}]}}}`

// The contract's example parses, wrapped or bare, and comes back as it
// went; anything the contract does not name is refused.
func TestParseExecutionPolicyReadsTheContract(t *testing.T) {
	for name, raw := range map[string]string{"wrapped": contractPolicy, "bare": strings.TrimSuffix(strings.TrimPrefix(contractPolicy, `{"policy": `), "}")} {
		t.Run(name, func(t *testing.T) {
			policy, err := ParseExecutionPolicy([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if policy.Level() != domain.PolicyObserve || len(policy.Allow()) != 2 || policy.Allow()[1].Arguments["path"] != "logs/**" || len(policy.Deny()) != 1 || policy.Deny()[0].Name != "local_exec" {
				t.Fatalf("%+v", policy)
			}
			wire, _ := json.Marshal(ExecutionPolicy(policy))
			if string(wire) != `{"level":"observe","tools":{"allow":[{"name":"observability.query_logs"},{"name":"local_read","arguments":{"path":"logs/**"}}],"deny":[{"name":"local_exec"}]}}` {
				t.Fatalf("%s", wire)
			}
		})
	}
	for name, raw := range map[string]string{
		"unknown field":      `{"level":"observe","tools":{},"budget":{}}`,
		"unknown rule field": `{"level":"observe","tools":{"allow":[{"name":"local_read","args":{}}]}}`,
		"unknown level":      `{"level":"autonomous","tools":{}}`,
		"bad rule":           `{"level":"observe","tools":{"deny":[{"name":""}]}}`,
		"not an object":      `[]`,
		"trailing data":      `{"level":"observe","tools":{}} {}`,
		"too large":          `{"level":"observe","tools":{"allow":[{"name":"` + strings.Repeat("a", MaxExecutionPolicyBytes) + `"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseExecutionPolicy([]byte(raw)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
