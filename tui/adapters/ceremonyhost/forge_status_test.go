package ceremonyhost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// execPort answers the exec tool like the local runtime does: a completed
// envelope whose stdout and stderr are the given texts, so Checks.Run cuts
// them to its tail.
type execPort struct{ stdout, stderr string }

func (p execPort) Execute(_ context.Context, _ domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
	encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"exit_code": 0, "stdout": p.stdout, "stderr": p.stderr}})
	return domain.ToolOutcome{Content: root.Text(encoded)}, nil
}

func TestForgeStatusReadsALargePullRequestView(t *testing.T) {
	var checks []string
	for i := 1; i <= 20; i++ {
		checks = append(checks, fmt.Sprintf(`{"name":"ci / unit and integration tests for the ceremony host adapters, matrix job %02d with a long descriptive name","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://github.com/underpass-ai/AXLR/actions/runs/%d/job/%d"}`, i, 1000+i, 2000+i))
	}
	view := `{"state":"OPEN","mergeStateStatus":"CLEAN","headRefOid":"h95","statusCheckRollup":[` + strings.Join(checks, ",") + `]}`
	if len(view) <= checkTail {
		t.Fatalf("fixture is %d bytes, must exceed the %d-byte check tail", len(view), checkTail)
	}
	// gh prints its release notice on stderr with exit 0; it is not part of the view.
	checksRunner := Checks{Tools: execPort{stdout: view, stderr: "\nA new release of gh is available: 2.80.0 → 2.81.0\n"}}
	status, err := Forge{Checks: checksRunner}.Status(context.Background(), "underpass-ai/AXLR", 95)
	if err != nil {
		t.Fatalf("a pull request view larger than the check tail must parse: %v", err)
	}
	if status.State != "OPEN" || status.MergeState != "CLEAN" || status.HeadSHA != "h95" || status.Passed != 20 || status.Pending != 0 || len(status.Failed) != 0 {
		t.Fatalf("status %+v", status)
	}
}
