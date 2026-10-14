package madesetup

import (
	"crypto/sha256"
	"encoding/hex"
)

// WorkGrantID names the exact action set below. MADE rejects reissuing an id
// with different actions, so a changed set needs a new id, never an edit.
// The grant issued carries the grantee's suffix (grantID): a MADE store that
// several installs share (one HOME, a config directory set up anew) holds
// one grant per work identity instead of refusing every identity but the
// first, which left a clean install without its session instances.
const WorkGrantID = "axlr-default-work-v2"

// grantID is the id of family's grant for grantee: the family and eight hex
// digits of the grantee's digest, so a conflict from MADE can only mean the
// family's actions changed under the same version.
// WorkGrantFor is the grant id the console issues to one work identity.
func WorkGrantFor(identity string) string {
	return grantID(WorkGrantID, identity)
}

func grantID(family, grantee string) string {
	digest := sha256.Sum256([]byte(grantee))
	return family + "-" + hex.EncodeToString(digest[:4])
}

// workGrantActions run, inspect and resume published ceremonies, report the
// host's status and handoffs, and let the model read, search, stream and
// design (decisions D6, H5 and H6 of the lifecycle integration design).
// They exclude guard approval, grant administration and definition
// publication (validate_ceremony_draft stays with the temporary install
// grant). v2 (13 October 2026) added the handoff, the draft explanation,
// diff and design, the successor plan and the reads the model may make; v1
// is left as it is.
var workGrantActions = []string{
	"apply_ceremony_transition", "assert_ceremony_reason", "cancel_ceremony", "claim_ceremony_step",
	"collect_ceremony_evidence", "complete_ceremony_step", "design_ceremony", "diff_ceremony_definitions",
	"explain_ceremony_draft", "generate_ceremony_report", "get_artifact", "get_ceremony_agent",
	"get_ceremony_definition", "get_ceremony_instance", "get_ceremony_transcript", "get_status",
	"inspect_ceremony_resume", "list_artifacts", "list_ceremony_agents", "list_ceremony_definitions",
	"list_ceremony_instances", "pause_ceremony", "plan_ceremony_successor", "read_artifact_chunk",
	"read_authorization_policy", "read_ceremony_events", "renew_ceremony_step_lease",
	"report_ceremony_agent_status", "resume_ceremony", "search_ceremony_instances", "start_ceremony_successor",
	"start_published_ceremony", "stream_ceremony", "verify_ceremony_journal",
}
