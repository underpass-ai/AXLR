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
	if guide.Role != domain.PluginRoleMemory || guide.Hidden == "" || guide.ContextArgument != "" || guide.MemoryWrite {
		t.Fatalf("kmp_guide %+v", guide)
	}
	write := Surface(ref("kmp", "kmp_write_memory"))
	if write.Hidden != "" || write.ContextArgument != "context_id" || !write.MemoryWrite {
		t.Fatalf("kmp_write_memory %+v", write)
	}
	if ask := Surface(ref("kmp", "kmp_ask")); ask.Hidden != "" || ask.ContextArgument != "context_id" || ask.MemoryWrite {
		t.Fatalf("kmp_ask %+v", ask)
	}
	if !strings.Contains(guide.Hidden, "axlr_recall") || !strings.Contains(guide.Hidden, "without context_id") {
		t.Fatalf("kmp_guide refusal: %s", guide.Hidden)
	}
	for _, verb := range []string{"made_start_published_ceremony", "made_start_ceremony", "made_claim_ceremony_step", "made_complete_ceremony_step", "made_apply_ceremony_transition", "made_cancel_ceremony", "made_pause_ceremony", "made_resume_ceremony", "made_run_ceremony", "made_run_ceremony_step", "made_approve_ceremony_guard", "made_defer_ceremony_guard", "made_issue_authorization_grant", "made_revoke_authorization_grant", "made_publish_ceremony_definition", "made_register_agent", "made_unregister_agent", "made_delete_contract", "made_begin_artifact_upload", "made_put_artifact_chunk", "made_report_ceremony_agent_status", "made_renew_ceremony_step_lease", "made_record_ceremony_host_handoff", "made_orchestrate", "made_deliberate", "made_create_council", "made_design_agentic_system", "made_lease_council_events", "made_pull_ceremony_events", "made_request_ceremony_intervention"} {
		if s := Surface(ref("made", verb)); s.Role != domain.PluginRoleOrchestrator || s.Hidden == "" || s.ContextArgument != "" {
			t.Fatalf("%s %+v", verb, s)
		}
	}
	for _, verb := range []string{"made_get_help", "made_discover_capabilities", "made_list_ceremony_definitions", "made_get_ceremony_instance", "made_read_ceremony_events", "made_stream_ceremony", "made_search_ceremony_instances", "made_inspect_ceremony_resume", "made_design_ceremony", "made_validate_ceremony_draft", "made_explain_ceremony_draft", "made_diff_ceremony_definitions", "made_render_agentic_system_diagram", "made_generate_ceremony_report", "made_verify_ceremony_journal"} {
		if s := Surface(ref("made", verb)); s.Hidden != "" {
			t.Fatalf("%s hidden: %s", verb, s.Hidden)
		}
	}
	if other := Surface(ref("github", "list_issues")); other != (domain.ToolSurface{}) {
		t.Fatalf("other plugin %+v", other)
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

// "No surgió nada durable que merezca registro en memoria; no grabo nada"
// closed a reply on 14 October 2026: the record sentence asked the model
// to say what it recorded or that nothing durable came out. Recording is
// silent now, in both forms.
func TestRecordGuidanceIsSilent(t *testing.T) {
	for _, text := range []string{kmpRecordGuidance, kmpRecordBridgedGuidance} {
		if strings.Contains(text, "say what you recorded") || !strings.Contains(text, "silently: never tell the person what you recorded or that nothing was") {
			t.Fatalf("record guidance %q", text)
		}
	}
}
