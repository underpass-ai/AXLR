package ceremonyhost

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// The KMP and MADE paragraphs of the model's orientation (decision D8 of
// the lifecycle integration design): the console runs both protocols, so
// the model is told what it gets and what it never does. With the
// session line they stay under 1 KB and identical across sessions, so
// the prompt's cached prefix keeps them.
const (
	kmpGuidance = "Memory (KMP): the console recalled the workspace's memory in the [AXLR · recall] note of your first prompt and records every turn itself; you never register, wake or page KMP for bookkeeping. For a question use axlr_recall; UNKNOWN is a valid answer, never invent a memory. Save a KMP about the person or the repository's instructions name with axlr_session.\n"
	// kmpRecordGuidance asks for the model's own records where it keeps
	// them; the bridged form serves a session without axlr_remember. Both
	// are bookkeeping the person never hears about: they asked the model
	// to "say what you recorded, or that nothing durable came out", and on
	// 14 October 2026 glm-5.3-flash closed a reply with "No surgió nada
	// durable que merezca registro en memoria; no grabo nada".
	kmpRecordGuidance        = "Record what the work settles: a decision, constraint, fix or outcome worth reusing goes to axlr_remember (always with summary_en) before your final answer, silently: never tell the person what you recorded or that nothing was. Never record transcripts, guesses or facts nothing settled.\n"
	kmpRecordBridgedGuidance = "Record what the work settles: a decision, constraint, fix or outcome worth reusing goes to kmp_write_memory through axlr_call_tool, under the session's exact about (axlr_session shows it) with its source evidence and a stable idempotency key, before your final answer, silently: never tell the person what you recorded or that nothing was. Never record transcripts, guesses or facts nothing settled.\n"
	madeGuidance             = "Ceremonies (MADE): the console runs them, this session included; you cannot start, claim or transition one. Read definitions and instances when needed; to run or publish one, ask the person.\n"
)

// Guidance is application.EngineGuidancePort for KMP and MADE.
type Guidance struct{}

var _ application.EngineGuidancePort = Guidance{}

// Paragraph words the orientation for one plugin; another plugin gets
// none.
func (Guidance) Paragraph(plugin root.PluginID, facts application.PluginGuidanceFacts) string {
	switch plugin {
	case memoryPluginID:
		text := kmpGuidance
		switch {
		case facts.RecordsMemory && facts.Remembers:
			text += kmpRecordGuidance
		case facts.RecordsMemory:
			text += kmpRecordBridgedGuidance
		}
		return text
	case orchestratorPluginID:
		return madeGuidance
	}
	return ""
}
