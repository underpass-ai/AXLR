package ceremonyhost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// madeAnswers answers MADE tools by name: a structured result, or a failed
// envelope with the message when the answer is an error.
func madeAnswers(answers map[string]any) memoryToolsFunc {
	return func(_ context.Context, id domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		answer := answers[string(id.Plugin.ToolName)]
		if err, failed := answer.(error); failed {
			encoded, _ := json.Marshal(map[string]any{"status": "failed", "error": map[string]any{"message": err.Error()}})
			return domain.ToolOutcome{Content: root.Text(encoded), IsError: true}, nil
		}
		encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(encoded)}, nil
	}
}

// A failed made_inspect_ceremony_resume used to be dropped: the view then
// said "no live claim of ours", and nothing showed MADE's error.
func TestEngineInspectSurfacesAFailedResumeInspection(t *testing.T) {
	engine := Engine{Tools: madeAnswers(map[string]any{
		"made_get_ceremony_instance":   map[string]any{"current_state": "PROPOSE", "claimable_step_ids": []any{}},
		"made_inspect_ceremony_resume": fmt.Errorf("journal is being compacted"),
	})}
	view, err := engine.Inspect(context.Background(), "axlr-x-1")
	if err != nil || view.State != "PROPOSE" {
		t.Fatalf("the instance itself was read: %+v %v", view, err)
	}
	if view.LiveErr == nil || !strings.Contains(view.LiveErr.Error(), "journal is being compacted") || len(view.Live) != 0 {
		t.Fatalf("the resume inspection's error is lost: %+v", view)
	}
}

// The host reports as the claim's owner: MADE accepts a status report only
// from it (13 October 2026), so the host agent id is the owner and the
// install and run travel as the incarnation.
func TestEngineReportsAsTheClaimOwner(t *testing.T) {
	var requests []map[string]any
	engine := Engine{Owner: "made-local-host-1-axlr-work", Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		var request map[string]any
		_ = json.Unmarshal(args.Bytes(), &request)
		requests = append(requests, map[string]any{"tool": string(id.Plugin.ToolName), "args": request})
		answer := map[string]any{}
		switch id.Plugin.ToolName {
		case "made_inspect_ceremony_resume":
			answer = map[string]any{"claims": []any{map[string]any{"claim_fence": "f1", "step_id": "turn", "phase": "live", "owner": "made-local-host-1-axlr-work", "operation_id": "op-1"}}}
		case "made_claim_ceremony_step":
			answer = map[string]any{"claim_fence": "f1"}
		}
		encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(encoded)}, nil
	})}
	ctx := context.Background()
	if fence, err := engine.Claim(ctx, "i", "turn", "k", 0); err != nil || fence != "f1" || requests[0]["args"].(map[string]any)["lease_owner_id"] != "made-local-host-1-axlr-work" {
		t.Fatalf("%q %v %+v", fence, err, requests[0])
	}
	if op, err := engine.OperationID(ctx, "i", "turn", "f1"); err != nil || op != "op-1" {
		t.Fatalf("%q %v", op, err)
	}
	if _, err := engine.OperationID(ctx, "i", "turn", "other"); err == nil {
		t.Fatal("an unknown fence has no operation id")
	}
	host := application.HostIdentity{Owner: "made-local-host-1-axlr-work", AgentID: "install1", Incarnation: "run1"}
	if err := engine.Report(ctx, application.HostReport{Instance: "i", Step: "turn", Fence: "f1", OperationID: "op-1", Status: "running", Activity: "turn", Summary: "turn 1", Execution: "s1", Worker: "s1", Role: "AXLR", Model: "m", Sequence: 1, Host: host}); err != nil {
		t.Fatal(err)
	}
	status := requests[len(requests)-1]["args"].(map[string]any)["status"].(map[string]any)
	if status["host_agent_id"] != "made-local-host-1-axlr-work" || status["claim_owner_id"] != "made-local-host-1-axlr-work" || status["host_agent_incarnation"] != "install1:run1" || status["operation_id"] != "op-1" || status["claim_fence"] != "f1" || status["execution_status"] != "running" || status["report_sequence"] != float64(1) || status["idempotency_key"] != "axlr:report:i:turn:1" || status["requested_model"] != "m" || status["role_id"] != "AXLR" || status["source"] != "host_report" || status["liveness"] != "fresh" {
		t.Fatalf("%+v", status)
	}
	if err := engine.Renew(ctx, "i", "turn", "f1", 0); err != nil {
		t.Fatal(err)
	}
	renew := requests[len(requests)-1]["args"].(map[string]any)
	if renew["lease_owner_id"] != "made-local-host-1-axlr-work" || renew["claim_fence"] != "f1" || renew["lease_ttl_ms"] != float64(claimLease) || renew["renewal_id"] == nil {
		t.Fatalf("%+v", renew)
	}
	if (Engine{}).owner() != actorID {
		t.Fatal("without a work identity the owner stays axlr")
	}
}

func TestEngineDeclaresAHandoffForTheOwner(t *testing.T) {
	var request map[string]any
	engine := Engine{Owner: "w", Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if id.Plugin.ToolName != "made_record_ceremony_host_handoff" {
			t.Fatalf("%+v", id)
		}
		_ = json.Unmarshal(args.Bytes(), &request)
		encoded, _ := json.Marshal(map[string]any{"status": "completed", "output": map[string]any{"structured_content": map[string]any{"recorded": true}}})
		return domain.ToolOutcome{Content: root.Text(encoded)}, nil
	})}
	if err := engine.Handoff(context.Background(), application.HostHandoff{Instance: "i", Step: "turn", Fence: "f1", Incarnation: "install:run1", Evidence: "run1 gone"}); err != nil {
		t.Fatal(err)
	}
	declaration := request["declaration"].(map[string]any)
	if request["ceremony_id"] != "i" || declaration["owner"] != "w" || declaration["incarnation"] != "install:run1" || declaration["state"] != "lost" || declaration["claim_fence"] != "f1" || declaration["evidence"] != "run1 gone" || declaration["id"] != "axlr:handoff:i:f1" {
		t.Fatalf("%+v", request)
	}
}
