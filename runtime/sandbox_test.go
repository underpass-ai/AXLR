package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/dto"
)

// A required sandbox that cannot run here refuses every command instead of
// running it unconfined, on every platform.
func TestARequiredSandboxThatIsUnavailableRefusesExec(t *testing.T) {
	e, err := newTestExecutor(t, Config{Root: t.TempDir(), Sandbox: &local.Sandbox{Unavailable: "bwrap is not installed"}})
	if err != nil {
		t.Fatal(err)
	}
	r := e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "r", Tool: "exec", Arguments: json.RawMessage(`{"program":"true"}`)})
	if r.Status != "rejected" || r.Error == nil || r.Error.Code != "sandbox_unavailable" || !strings.Contains(r.Error.Message, "bwrap is not installed") {
		t.Fatalf("%+v", r)
	}
}

// The probe at launch finds a missing or unusable bwrap before the model's
// first command does.
func TestTheSandboxProbeRefusesAMissingBwrap(t *testing.T) {
	for _, sandbox := range []local.Sandbox{{}, {Program: "/no/such/bwrap"}} {
		if err := local.ProbeSandbox(context.Background(), sandbox); err == nil {
			t.Fatalf("probe of %+v succeeded", sandbox)
		}
	}
}
