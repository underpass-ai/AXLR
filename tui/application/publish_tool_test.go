package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// publisherStub plays CatalogUseCase's publishing steps.
type publisherStub struct {
	draft     PublishDraft
	submitted []PublishDraft
}

func (p *publisherStub) Prepare(_ context.Context, _ string, request PublishRequest) (PublishDraft, error) {
	draft := p.draft
	draft.Tool.Name = request.Name
	if request.Version != "" {
		draft.Metadata.Version = request.Version
	}
	draft.Missing = missingPublishFields(draft.Metadata)
	return draft, nil
}

func (p *publisherStub) Bundle(_ context.Context, draft PublishDraft) (PublishBundle, error) {
	return PublishBundle{KeyID: "k1"}, nil
}

func (p *publisherStub) Submit(_ context.Context, draft PublishDraft, _ PublishBundle) (VersionReport, error) {
	p.submitted = append(p.submitted, draft)
	return VersionReport{Name: draft.Tool.Name, Version: draft.Metadata.Version, Status: "scanned"}, nil
}

func completeDraft() PublishDraft {
	return PublishDraft{Catalog: "axlr", Tests: []byte(`[{"name":"one","input":{},"expect":{"exit":0}}]`), Metadata: PublishMetadata{Version: "1.0.0", Category: "data", License: "MIT", Tags: []string{"image"}}}
}

// On 20 October 2026 the agent could not publish a tool the person asked
// for: axlr_publish_tool uploads it after the card, refuses before the card
// what the registry could never approve, and is never approved
// automatically, autonomy included.
func TestPublishToolUploadsAndRefusesWhatCannotBeApproved(t *testing.T) {
	publisher := &publisherStub{draft: completeDraft()}
	host := HostToolUseCase{Publisher: publisher}
	s := turnSession(t)
	outcome, err := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationPublishTool), object(t, `{"name":"image_frequency","why":"publicarla en el catálogo"}`))
	if err != nil || outcome.IsError || len(publisher.submitted) != 1 {
		t.Fatalf("outcome %s, %v, submitted %d", outcome.Content, err, len(publisher.submitted))
	}
	var result map[string]any
	if json.Unmarshal([]byte(outcome.Content), &result) != nil || result["published"] != "image_frequency" || result["version"] != "1.0.0" || result["status"] != "scanned" {
		t.Fatalf("result %s", outcome.Content)
	}

	noTests := completeDraft()
	noTests.Tests = nil
	noCatalog := completeDraft()
	noCatalog.Metadata = PublishMetadata{}
	for name, test := range map[string]struct {
		draft PublishDraft
		want  string
	}{
		"no tests":      {noTests, "has no tests.json"},
		"empty catalog": {noCatalog, "lacks version, category, license"},
	} {
		err := publishArgumentError(context.Background(), &publisherStub{draft: test.draft}, s, PublishTool(), object(t, `{"name":"image_frequency","why":"x"}`))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := publishArgumentError(context.Background(), nil, s, PublishTool(), object(t, `{"name":"x1","why":"x"}`)); err == nil {
		t.Fatal("a console without a registry client let the call reach its card")
	}

	if automaticallyApproves(allowEverything{}, PublishTool().Identity) {
		t.Fatal("a publication was approved without the person's card")
	}
	if !RequiresWhy(PublishTool()) {
		t.Fatal("a publication needs a why like every side effect")
	}
	if verdict, _ := domain.ModeReview.Judge(PublishTool().Identity, object(t, `{"name":"x1"}`)); verdict != domain.VerdictDeny {
		t.Fatal("review mode published a tool")
	}
}
