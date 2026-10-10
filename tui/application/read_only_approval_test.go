package application

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026, with /autonomy off, every local_search and local_read
// of claude-haiku-5.5 raised a card: a "where is X" question cost three to
// five keypresses before the model could look. Reading the workspace changes
// nothing, so it needs no card, whatever autonomy says.
func TestReadOnlyWorkspaceToolsNeedNoCardWithAutonomyOff(t *testing.T) {
	never := approvalFunc(func(domain.ToolIdentity) bool { return false })
	args, _ := root.NewJSONObject([]byte(`{"path":"README.md"}`))
	for _, op := range []string{"read", "search", "list"} {
		id := localIdentity(op)
		if !automaticallyApproves(never, id) || !automaticallyApproves(nil, id) {
			t.Fatalf("local_%s needs a card with autonomy off", op)
		}
		for _, mode := range []domain.WorkMode{domain.ModeNormal, domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
			if !approvesInMode(never, mode, id, args) {
				t.Fatalf("local_%s needs a card in %s mode", op, mode)
			}
		}
	}
	for _, op := range []string{"write", "edit", "exec"} {
		if automaticallyApproves(never, localIdentity(op)) || automaticallyApproves(nil, localIdentity(op)) {
			t.Fatalf("local_%s approved without the person or autonomy", op)
		}
	}
}
