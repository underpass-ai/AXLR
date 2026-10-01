package madesetup

// WorkGrantID names the exact action set below. MADE rejects reissuing an id
// with different actions, so a changed set needs a new id, never an edit.
const WorkGrantID = "axlr-default-work-v1"

// workGrantActions run, inspect and resume published ceremonies. They exclude
// guard approval, grant administration and definition publication.
var workGrantActions = []string{
	"apply_ceremony_transition", "assert_ceremony_reason", "cancel_ceremony", "claim_ceremony_step",
	"collect_ceremony_evidence", "complete_ceremony_step", "generate_ceremony_report", "get_artifact",
	"get_ceremony_agent", "get_ceremony_definition", "get_ceremony_instance", "get_ceremony_transcript",
	"inspect_ceremony_resume", "list_artifacts", "list_ceremony_agents", "list_ceremony_definitions",
	"list_ceremony_instances", "pause_ceremony", "read_artifact_chunk", "read_authorization_policy",
	"read_ceremony_events", "renew_ceremony_step_lease", "report_ceremony_agent_status", "resume_ceremony",
	"search_ceremony_instances", "start_published_ceremony",
}
