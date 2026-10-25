package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// fakeDeliverables records what axlr_deliver asked of the workspace.
type fakeDeliverables struct {
	read, written string
	content       []byte
	err           error
}

func (f *fakeDeliverables) DeliverFile(_ context.Context, _, path string) (DeliveredFile, error) {
	f.read = path
	return DeliveredFile{Size: 7, SHA256: strings.Repeat("f", 64)}, f.err
}

func (f *fakeDeliverables) WriteDeliverable(_ context.Context, _, target string, content []byte) (DeliveredFile, error) {
	f.written, f.content = target, content
	return DeliveredFile{Size: int64(len(content)), SHA256: strings.Repeat("a", 64)}, f.err
}

// A file is delivered where it is; inline content is written under the
// session's deliverables folder. The result names the artifact, never its
// content, and the execution's result reads it back from the outcome.
func TestHostDeliverHandsOverAnArtifact(t *testing.T) {
	session := hostSession(t, "done")
	port := &fakeDeliverables{}
	use := HostToolUseCase{Deliverables: port}
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationDeliver)
	args, _ := root.NewJSONObject([]byte(`{"why":"the task asked for it","name":"report.md","media_type":"text/markdown","description":"What failed","path":"out/report.md"}`))
	outcome, err := use.Execute(context.Background(), session, identity, args)
	if err != nil || outcome.IsError || port.read != "out/report.md" {
		t.Fatalf("%+v %v %q", outcome, err, port.read)
	}
	var delivered domain.DeliveredArtifact
	if err := json.Unmarshal([]byte(outcome.Content), &delivered); err != nil || delivered.Artifact.Reference != "out/report.md" || delivered.Artifact.Source != domain.ArtifactFile || delivered.Artifact.Size != 7 {
		t.Fatalf("%s %v", outcome.Content, err)
	}
	args, _ = root.NewJSONObject([]byte(`{"name":"plan.md","media_type":"text/markdown","description":"The plan","content":"# Plan\n"}`))
	outcome, _ = use.Execute(context.Background(), session, identity, args)
	want := ".axlr/deliverables/" + string(session.ID()) + "/plan.md"
	if outcome.IsError || port.written != want || string(port.content) != "# Plan\n" || strings.Contains(string(outcome.Content), "# Plan") {
		t.Fatalf("%s %q", outcome.Content, port.written)
	}
	record := domain.PendingTool{Call: root.ToolCall{ID: "d1", Name: HostDeliverName}, Decision: domain.DecisionAutoApprove, Outcome: &outcome}
	if artifact, ok := domain.ArtifactOf(session.ID(), record); !ok || artifact.Reference != want || artifact.Source != domain.ArtifactInline || artifact.Call != "d1" {
		t.Fatalf("%+v %v", artifact, ok)
	}
	port.err = errors.New("out/x.md leaves the workspace")
	if outcome, _ := use.Execute(context.Background(), session, identity, args); !outcome.IsError || !strings.Contains(string(outcome.Content), "leaves the workspace") {
		t.Fatalf("%s", outcome.Content)
	}
	if outcome, _ := (HostToolUseCase{}).Execute(context.Background(), session, identity, args); !outcome.IsError {
		t.Fatalf("delivered without a port: %s", outcome.Content)
	}
}

// A delivery by path is approved as a read, without a card; inline content
// is approved as local_write would be, and refused before a card when its
// arguments cannot deliver.
func TestDeliveriesAreApprovedAsReadsOrWrites(t *testing.T) {
	never := approvalFunc(func(domain.ToolIdentity) bool { return false })
	writes := approvalFunc(func(id domain.ToolIdentity) bool { return id == localIdentity("write") })
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationDeliver)
	byPath, _ := root.NewJSONObject([]byte(`{"name":"r.json","media_type":"application/json","description":"d","path":"r.json"}`))
	inline, _ := root.NewJSONObject([]byte(`{"name":"plan.md","media_type":"text/markdown","description":"d","content":"x"}`))
	if !approvesInMode(never, domain.ModeNormal, identity, byPath) || !approvesInMode(nil, domain.ModeReview, identity, byPath) {
		t.Fatal("a file by path needs a card")
	}
	if approvesInMode(never, domain.ModeNormal, identity, inline) || approvesInMode(nil, domain.ModeNormal, identity, inline) {
		t.Fatal("inline content approved without the person or a rule for writes")
	}
	if !approvesInMode(writes, domain.ModeNormal, identity, inline) || approvesInMode(writes, domain.ModeReview, identity, inline) {
		t.Fatal("inline content is approved as local_write is")
	}
	if err := deliverArgumentError(DeliverTool(), byPath); err != nil {
		t.Fatal(err)
	}
	bad, _ := root.NewJSONObject([]byte(`{"name":"../x","media_type":"text/plain","description":"d","content":"x"}`))
	if err := deliverArgumentError(DeliverTool(), bad); err == nil || !strings.HasPrefix(err.Error(), "axlr_deliver: name must be") {
		t.Fatalf("%v", err)
	}
	if !RequiresWhy(DeliverTool()) {
		t.Fatal("axlr_deliver takes the why every hand-over takes")
	}
}
