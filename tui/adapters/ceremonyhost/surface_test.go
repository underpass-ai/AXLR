package ceremonyhost

import (
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func ref(plugin, tool string) root.PluginRef {
	return root.PluginRef{PluginID: root.PluginID(plugin), ToolName: root.PluginToolName(tool)}
}

// KMP's guide and MADE's write verbs are the console's; KMP's calls carry
// the session's context; the memory write is marked; reads of MADE and
// the tools of other plugins are the model's.
func TestSurfaceKeepsTheEnginesProtocolsToTheConsole(t *testing.T) {
	guide := Surface(ref("kmp", "kmp_guide"))
	if guide.Role != domain.PluginRoleMemory || guide.Hidden == "" || guide.ContextArgument != "" || guide.MemoryWrite || !guide.Entry {
		t.Fatalf("kmp_guide %+v", guide)
	}
	write := Surface(ref("kmp", "kmp_write_memory"))
	if write.Hidden != "" || write.ContextArgument != "context_id" || !write.MemoryWrite || !write.Entry {
		t.Fatalf("kmp_write_memory %+v", write)
	}
	if ask := Surface(ref("kmp", "kmp_ask")); ask.Hidden != "" || ask.ContextArgument != "context_id" || ask.MemoryWrite || ask.Entry {
		t.Fatalf("kmp_ask %+v", ask)
	}
	for _, verb := range []string{"made_claim_ceremony_step", "made_start_ceremony", "made_issue_authorization_grant", "made_publish_ceremony_definition", "made_approve_ceremony_guard"} {
		if s := Surface(ref("made", verb)); s.Role != domain.PluginRoleOrchestrator || s.Hidden == "" || s.ContextArgument != "" {
			t.Fatalf("%s %+v", verb, s)
		}
	}
	for _, verb := range []string{"made_get_ceremony_instance", "made_list_ceremony_definitions", "made_read_ceremony_events", "made_stream_ceremony", "made_search_ceremony_instances", "made_inspect_ceremony_resume", "made_get_help", "made_design_ceremony", "made_validate_ceremony_draft", "made_generate_ceremony_report", "made_verify_ceremony_journal"} {
		if s := Surface(ref("made", verb)); s.Hidden != "" {
			t.Fatalf("%s hidden: %s", verb, s.Hidden)
		}
	}
	if other := Surface(ref("github", "list_issues")); other != (domain.ToolSurface{}) {
		t.Fatalf("other plugin %+v", other)
	}
	if s := Surface(ref("docs", "docs_guide")); s.Role != domain.PluginRoleNone || !s.Entry {
		t.Fatalf("guide of another plugin %+v", s)
	}
}

// The paragraphs stay under 1 KB with the session line so the prompt's
// cached prefix keeps them; the record sentence follows the session.
func TestGuidanceParagraphsStayShortAndFollowTheSession(t *testing.T) {
	if size := len(kmpGuidance) + len(kmpRecordGuidance) + len(madeGuidance) + len(application.SessionTitleGuidance); size > 1024 {
		t.Fatalf("engine guidance is %d bytes", size)
	}
	g := Guidance{}
	plain := g.Paragraph("kmp", application.PluginGuidanceFacts{})
	own := g.Paragraph("kmp", application.PluginGuidanceFacts{RecordsMemory: true, Remembers: true})
	bridged := g.Paragraph("kmp", application.PluginGuidanceFacts{RecordsMemory: true})
	if plain != kmpGuidance || !strings.Contains(own, "axlr_remember") || !strings.Contains(bridged, "axlr_call_tool") || strings.Contains(own, "axlr_call_tool") {
		t.Fatalf("kmp paragraphs:\n%s\n%s\n%s", plain, own, bridged)
	}
	if g.Paragraph("made", application.PluginGuidanceFacts{}) != madeGuidance || g.Paragraph("github", application.PluginGuidanceFacts{}) != "" {
		t.Fatal("made or other paragraph")
	}
}
